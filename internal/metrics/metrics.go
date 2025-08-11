package metrics

import "github.com/prometheus/client_golang/prometheus"

var (
	WSMessagesTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{Name: "im_ws_messages_total", Help: "WS上行消息数"},
		[]string{"action"},
	)
	MessageSendLatency = prometheus.NewHistogram(
		prometheus.HistogramOpts{Name: "im_send_latency_ms", Help: "消息发送端到端延迟(近似)", Buckets: prometheus.LinearBuckets(5, 5, 20)},
	)
)

func Init() {
	prometheus.MustRegister(WSMessagesTotal)
	prometheus.MustRegister(MessageSendLatency)
}
