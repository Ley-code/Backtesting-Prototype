package api

import (
	"net/http"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

type Server struct {
	pool   *Pool
	webDir string
}

func NewServer(pool *Pool, webDir string) *Server {
	return &Server{pool: pool, webDir: webDir}
}

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

	r.StaticFile("/", s.webDir+"/index.html")
	r.Static("/static", s.webDir)

	return r
}

func (s *Server) handleOptions(c *gin.Context) {
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
				"value": "ema_crossover", "label": "EMA Crossover",
				"params": []gin.H{
					{"key": "fast", "label": "Fast EMA (bars)", "default": 10, "min": 2, "max": 200},
					{"key": "slow", "label": "Slow EMA (bars)", "default": 30, "min": 3, "max": 400},
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
			{
				"value": "bollinger_bounce", "label": "Bollinger Bounce",
				"params": []gin.H{
					{"key": "period", "label": "Period (bars)", "default": 20, "min": 5, "max": 100},
					{"key": "std_dev", "label": "Std dev (×10)", "default": 20, "min": 10, "max": 40},
				},
			},
			{
				"value": "breakout", "label": "Breakout",
				"params": []gin.H{
					{"key": "lookback", "label": "Lookback (bars)", "default": 20, "min": 5, "max": 200},
				},
			},
			{
				"value": "momentum_pct", "label": "Momentum %",
				"params": []gin.H{
					{"key": "lookback", "label": "Lookback (bars)", "default": 20, "min": 5, "max": 200},
					{"key": "buy_pct", "label": "Buy threshold (%)", "default": 10, "min": 1, "max": 50},
					{"key": "sell_pct", "label": "Sell from peak (%)", "default": 5, "min": 1, "max": 50},
				},
			},
			{
				"value": "tp_sl", "label": "Take Profit / Stop Loss",
				"params": []gin.H{
					{"key": "fast", "label": "Fast MA (bars)", "default": 10, "min": 2, "max": 200},
					{"key": "slow", "label": "Slow MA (bars)", "default": 30, "min": 3, "max": 400},
					{"key": "tp_pct", "label": "Take profit (%)", "default": 10, "min": 1, "max": 100},
					{"key": "sl_pct", "label": "Stop loss (%)", "default": 5, "min": 1, "max": 50},
				},
			},
		},
	})
}

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

func (s *Server) handleGet(c *gin.Context) {
	job, ok := s.pool.Get(c.Param("id"))
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
		return
	}
	c.JSON(http.StatusOK, job)
}
