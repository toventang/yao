package robot

import (
	"context"

	"github.com/yaoapp/yao/agent/robot/cache"
	"github.com/yaoapp/yao/agent/robot/dedup"
	"github.com/yaoapp/yao/agent/robot/events/integrations"
	"github.com/yaoapp/yao/agent/robot/events/integrations/telegram"
	"github.com/yaoapp/yao/agent/robot/executor"
	"github.com/yaoapp/yao/agent/robot/logger"
	"github.com/yaoapp/yao/agent/robot/manager"
	"github.com/yaoapp/yao/agent/robot/plan"
	"github.com/yaoapp/yao/agent/robot/pool"
	"github.com/yaoapp/yao/agent/robot/store"
	robottypes "github.com/yaoapp/yao/agent/robot/types"
)

var (
	log = logger.New("robot")

	globalManager    *manager.Manager
	globalCache      *cache.Cache
	globalPool       *pool.Pool
	globalDedup      *dedup.Dedup
	globalStore      *store.Store
	globalExecutor   executor.Executor
	globalPlan       *plan.Plan
	globalDispatcher *integrations.Dispatcher
)

// Init initializes the robot agent system
func Init() error {
	globalCache = cache.New()
	globalDedup = dedup.New()
	globalStore = store.New()
	globalPool = pool.New()
	globalExecutor = executor.New()
	globalManager = manager.New()
	globalPlan = plan.New()

	// Load robots into cache from database before starting dispatcher
	rCtx := robottypes.NewContext(context.Background(), nil)
	if err := globalCache.Load(rCtx); err != nil {
		log.Warn("robot.Init: cache load failed (will rely on config events): %v", err)
	}

	adapters := map[string]integrations.Adapter{
		"telegram": telegram.NewAdapter(),
	}
	globalDispatcher = integrations.NewDispatcher(globalCache, adapters)
	if err := globalDispatcher.Start(context.Background()); err != nil {
		return err
	}

	return nil
}

// Shutdown gracefully shuts down the robot agent system
func Shutdown() error {
	if globalDispatcher != nil {
		globalDispatcher.Stop()
	}
	return nil
}

// Manager returns the global manager instance
func Manager() *manager.Manager {
	return globalManager
}
