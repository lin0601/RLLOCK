package main

import (
	"RLLOCK"
	"context"
	"fmt"
	"time"
)

func main() {
	config := RLLOCK.CreateRedisConfig("host:port", 2, "", 500, 0, 0)
	pool, err := RLLOCK.ConnectRedisPool(config)
	if err != nil {
		fmt.Print("err: %s\n", err.Error())
		return
	}
	lockFactory := RLLOCK.CreateLock(pool)
	lockFactory.SetKeepAlive(true)
	lockFactory.SetLockRetryCount(3)
	lockFactory.SetLockRetryDelayMS(500)

	lockObj, err := lockFactory.Lock(context.Background(), "ceshi123", 1*1000)
	if err != nil {
		fmt.Print("加锁失败: %s\n", err.Error())
		return
	}

	if lockObj == nil {
		fmt.Print("加锁失败\n")
		return
	}

	time.Sleep(2 * time.Second)

	defer func() {
		_, err = lockFactory.UnLock(context.Background(), lockObj)

		if err != nil {
			fmt.Print("解锁失败: %s \n", err.Error())
			return
		}
	}()
}
