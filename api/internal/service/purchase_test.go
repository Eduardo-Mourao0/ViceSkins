package service

import (
	"testing"
	"time"
)

func TestRetryDelay(t *testing.T) {
	cases := []struct {
		attempt int32
		want    time.Duration
	}{
		{0, 5 * time.Second}, // defensivo: tentativa inválida usa o mínimo
		{1, 5 * time.Second},
		{2, 10 * time.Second},
		{3, 20 * time.Second},
		{6, 160 * time.Second},
		{7, 5 * time.Minute}, // 320s passaria do teto
		{40, 5 * time.Minute},
	}
	for _, c := range cases {
		if got := retryDelay(c.attempt); got != c.want {
			t.Errorf("retryDelay(%d) = %s, want %s", c.attempt, got, c.want)
		}
	}
}
