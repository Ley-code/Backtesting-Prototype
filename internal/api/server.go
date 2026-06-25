package api

import (
	"net/http"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

// Server wires the HTTP routes to the worker pool.
type Server struct {
	pool   *Pool
	webDir string
}

func NewServer(pool *Pool, webDir string) *Server {
	return &Server{pool: pool, webDir: webDir}
}

// Router builds the Gin engine: a small REST API plus the static single-page UI.
func (s *Server) Router() *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()
	r.Use(cors.Default())

	api := r.Group("/api")
	{
		api.GET("/options", s.handleOptions)
		api.POST("/backtests", s.handleSubmit)
		api.GET("/backtests/:id", s.handleGet)
	}

	// Serve the frontend.
	r.StaticFile("/", s.webDir+"/index.html")
	r.Static("/static", s.webDir)

	return r
}

// handleOptions tells the UI the fixed prototype surface (products/timeframes/strategies).
func (s *Server) handleOptions(c *gin.Context) {
	// Each strategy advertises its tunable params (key, label, default, min,
	// max) so the UI can render the right inputs dynamically. Periods are in
	// BARS — what they mean in time depends on the chosen timeframe.
	c.JSON(http.StatusOK, gin.H{
		"symbols":   []string{"BTCUSDT", "ETHUSDT"},
		"intervals": []gin.H{{"value": "1", "label": "1 min"}, {"value": "5", "label": "5 min"}, {"value": "15", "label": "15 min"}},
		"strategies": []gin.H{
			{
				"value": "ma_crossover", "label": "MA Crossover",
				"params": []gin.H{
					{"key": "fast", "label": "Fast MA (bars)", "default": 10, "min": 2, "max": 200},
					{"key": "slow", "label": "Slow MA (bars)", "default": 30, "min": 3, "max": 400},
				},
			},
			{
				"value": "rsi_reversion", "label": "RSI Mean-Reversion",
				"params": []gin.H{
					{"key": "period", "label": "RSI Period (bars)", "default": 14, "min": 2, "max": 100},
					{"key": "oversold", "label": "Oversold (<)", "default": 30, "min": 1, "max": 49},
					{"key": "overbought", "label": "Overbought (>)", "default": 70, "min": 51, "max": 99},
				},
			},
		},
	})
}

// handleSubmit enqueues a backtest and returns its job id (202 Accepted). The
// async model is what exercises the worker pool — the client then polls GET.
func (s *Server) handleSubmit(c *gin.Context) {
	var req BacktestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := validate(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	id, ok := s.pool.Submit(req)
	if !ok {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "backtest queue is full, please retry shortly"})
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"id": id, "status": StatusQueued})
}

// handleGet returns the current job state (and the result once done).
func (s *Server) handleGet(c *gin.Context) {
	job, ok := s.pool.Get(c.Param("id"))
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
		return
	}
	c.JSON(http.StatusOK, job)
}
