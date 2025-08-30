package utils

import (
	"os"
	"strconv"
	"time"
)

func GetEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func GetEnvAsInt(key string, defaultValue int) int {
	strVal := GetEnv(key, "")
	if strVal == "" {
		return defaultValue
	}
	if intVal, err := strconv.Atoi(strVal); err == nil {
		return intVal
	}
	return defaultValue
}

func GetEnvAsInt64(key string, defaultValue int64) int64 {
	strVal := GetEnv(key, "")
	if strVal == "" {
		return defaultValue
	}
	if intVal, err := strconv.ParseInt(strVal, 10, 64); err == nil {
		return intVal
	}
	return defaultValue
}

func GetEnvAsDuration(key string, defaultValue time.Duration) time.Duration {
	strVal := GetEnv(key, "")
	if strVal == "" {
		return defaultValue
	}
	if duration, err := time.ParseDuration(strVal); err == nil {
		return duration
	}
	return defaultValue
}

func GetEnvAsBool(key string, defaultValue bool) bool {
	strVal := GetEnv(key, "")
	if strVal == "" {
		return defaultValue
	}
	if boolVal, err := strconv.ParseBool(strVal); err == nil {
		return boolVal
	}
	return defaultValue
}
