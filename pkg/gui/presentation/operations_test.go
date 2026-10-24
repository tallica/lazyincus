package presentation

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestOperationDuration(t *testing.T) {
	assert.Equal(t, "87ms", OperationDuration(87*time.Millisecond+400*time.Microsecond))
	assert.Equal(t, "1.5s", OperationDuration(1520*time.Millisecond))
	assert.Equal(t, "2m5s", OperationDuration(125*time.Second+300*time.Millisecond))
}
