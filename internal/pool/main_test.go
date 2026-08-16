package pool

import (
	"os"
	"testing"

	"go.uber.org/zap"

	"github.com/Beardsoft/GoPool/internal/logger"
)

func TestMain(m *testing.M) {
	logger.Logger = zap.NewNop()
	os.Exit(m.Run())
}
