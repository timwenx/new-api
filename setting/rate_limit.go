package setting

import (
	"fmt"
	"math"
	"sync"

	"github.com/QuantumNous/new-api/common"
)

var ModelRequestRateLimitEnabled = false
var ModelRequestRateLimitDurationMinutes = 1
var ModelRequestRateLimitCount = 0
var ModelRequestRateLimitSuccessCount = 1000
var modelRequestConcurrencyLimit = 3
var ModelRequestRateLimitGroup = map[string][]int{}
var ModelRequestRateLimitMutex sync.RWMutex

func ModelRequestRateLimitGroup2JSONString() string {
	ModelRequestRateLimitMutex.RLock()
	defer ModelRequestRateLimitMutex.RUnlock()

	jsonBytes, err := common.Marshal(ModelRequestRateLimitGroup)
	if err != nil {
		common.SysLog("error marshalling model ratio: " + err.Error())
	}
	return string(jsonBytes)
}

func UpdateModelRequestRateLimitGroupByJSONString(jsonStr string) error {
	modelRequestRateLimitGroup := make(map[string][]int)
	if err := common.UnmarshalJsonStr(jsonStr, &modelRequestRateLimitGroup); err != nil {
		return err
	}
	if err := validateModelRequestRateLimitGroup(modelRequestRateLimitGroup); err != nil {
		return err
	}

	ModelRequestRateLimitMutex.Lock()
	defer ModelRequestRateLimitMutex.Unlock()
	ModelRequestRateLimitGroup = modelRequestRateLimitGroup
	return nil
}

func GetGroupRateLimit(group string) (totalCount, successCount int, found bool) {
	ModelRequestRateLimitMutex.RLock()
	defer ModelRequestRateLimitMutex.RUnlock()

	if ModelRequestRateLimitGroup == nil {
		return 0, 0, false
	}

	limits, found := ModelRequestRateLimitGroup[group]
	if !found {
		return 0, 0, false
	}
	return limits[0], limits[1], true
}

func GetGroupConcurrencyLimit(group string) int {
	ModelRequestRateLimitMutex.RLock()
	defer ModelRequestRateLimitMutex.RUnlock()

	limits, found := ModelRequestRateLimitGroup[group]
	if found && len(limits) == 3 {
		return limits[2]
	}
	return modelRequestConcurrencyLimit
}

func GetModelRequestConcurrencyLimit() int {
	ModelRequestRateLimitMutex.RLock()
	defer ModelRequestRateLimitMutex.RUnlock()
	return modelRequestConcurrencyLimit
}

func SetModelRequestConcurrencyLimit(limit int) {
	ModelRequestRateLimitMutex.Lock()
	defer ModelRequestRateLimitMutex.Unlock()
	modelRequestConcurrencyLimit = limit
}

func CheckModelRequestRateLimitGroup(jsonStr string) error {
	checkModelRequestRateLimitGroup := make(map[string][]int)
	if err := common.UnmarshalJsonStr(jsonStr, &checkModelRequestRateLimitGroup); err != nil {
		return err
	}
	return validateModelRequestRateLimitGroup(checkModelRequestRateLimitGroup)
}

func validateModelRequestRateLimitGroup(groups map[string][]int) error {
	for group, limits := range groups {
		if len(limits) != 2 && len(limits) != 3 {
			return fmt.Errorf("group %s must contain [max requests, max success] or [max requests, max success, max concurrency]", group)
		}
		if limits[0] < 0 || limits[1] < 1 {
			return fmt.Errorf("group %s has negative rate limit values: [%d, %d]", group, limits[0], limits[1])
		}
		if len(limits) == 3 && limits[2] < 0 {
			return fmt.Errorf("group %s has negative concurrency limit: %d", group, limits[2])
		}
		for _, limit := range limits {
			if limit > math.MaxInt32 {
				return fmt.Errorf("group %s has a limit greater than 2147483647", group)
			}
		}
	}

	return nil
}
