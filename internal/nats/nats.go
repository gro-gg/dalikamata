package nats

import "fmt"

const (
	DefaultHost = "0.0.0.0"
	DefaultPort = 4222
)

func NATSConnectionString(host string, port int) string {
	return fmt.Sprintf("nats://%s:%d", host, port)
}
