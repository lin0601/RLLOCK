package RLLOCK

import (
	"context"
	"errors"
	"fmt"
	"github.com/gofrs/uuid"
	"github.com/gomodule/redigo/redis"
	"sync"
	"time"
)

const (
	//默认重试次数
	DefaultLockRetryCount = 3
	//默认没枷锁的重试次数
	DefaultUnLockRetryCount = 3
	//默认续命重试次数
	DefaultContinuedRetryCount = 3
	//重试的延迟
	DefaultLockRetryDelayMS = 500
	//未加锁的重试延迟
	DefaultUnLockRetryDelayMS = 500
	//默认续命时间间隔
	DefaultContinuedTime = 500
)

var (
	mutexLock       sync.Mutex
	luaScriptDelete = redis.NewScript(1, `if redis.call('get',   KEYS[1]) == ARGV[1] then return redis.call('del', KEYS[1]) else return 0 end`)
	luaScriptSetnx  = redis.NewScript(1, `if redis.call('setnx', KEYS[1], ARGV[1]) == 1 then return redis.call('PEXPIRE',KEYS[1],ARGV[2]) else return 0 end`)
	luaScriptExpire = redis.NewScript(2, `if redis.call('get',   KEYS[1]) == ARGV[1] then return redis.call('PEXPIRE', KEYS[2], ARGV[2]) else return 0 end`)
)

//获取随机值
func GetRandomValue() (valueGUID string) {
	uuidData, _ := uuid.NewV4()
	return uuidData.String()
}

// 锁工厂
type lockFactory struct {
	pool                *redis.Pool
	lockRetryCount      int32
	lockRetryDelayMS    int64
	unLockRetryCount    int32
	unLockRetryDelayMS  int64
	continuedRetryCount int32
	isKeepAlive         bool
}

// 新建一个redis锁对象
func CreateLock(pool *redis.Pool) *lockFactory {
	return &lockFactory{
		pool:                pool,
		lockRetryCount:      DefaultLockRetryCount,
		lockRetryDelayMS:    DefaultLockRetryDelayMS,
		unLockRetryCount:    DefaultUnLockRetryCount,
		unLockRetryDelayMS:  DefaultUnLockRetryDelayMS,
		continuedRetryCount: DefaultContinuedRetryCount,
	}
}

// 可续命锁对象
type RLLock struct {
	CreateTime      int64
	ContinuedTime   int   //持续时间
	ContinuedTimeMS int64 //续命时间（ms）
	UnlockTime      int64
	KeyName         string //键
	RandomKey       string //随机值
	IsUnlock        bool
	RedisChan       chan struct{}
	ChanIsNoIn      bool
	IsLive          bool
}

// 参数的设置与获取
func (s *lockFactory) SetLockRetryCount(lockRetryCount int32) {
	if lockRetryCount < 0 {
		s.lockRetryCount = DefaultLockRetryCount
	} else {
		s.lockRetryCount = lockRetryCount
	}
}
func (s *lockFactory) GetLockRetryCount() int32 {
	return s.lockRetryCount
}

func (s *lockFactory) SetUnLockRetryCount(unLockRetryCount int32) {
	if unLockRetryCount < 0 {
		s.unLockRetryCount = DefaultUnLockRetryCount
	} else {
		s.unLockRetryCount = unLockRetryCount
	}

}
func (s *lockFactory) GetUnLockRetryCount() int32 {
	return s.unLockRetryCount
}

func (s *lockFactory) SetKeepAlive(isKeepAlive bool) {
	s.isKeepAlive = isKeepAlive
}
func (s *lockFactory) GetKeepAlive() bool {
	return s.isKeepAlive
}

func (s *lockFactory) SetLockRetryDelayMS(lockRetryDelayMS int64) {
	if lockRetryDelayMS < 0 {
		s.lockRetryDelayMS = DefaultLockRetryDelayMS
	} else {
		s.lockRetryDelayMS = lockRetryDelayMS
	}
}
func (s *lockFactory) GetLockRetryDelayMS() int64 {
	return s.lockRetryDelayMS
}

func (s *lockFactory) SetUnLockRetryDelayMS(unLockRetryDelayMS int64) {
	if unLockRetryDelayMS < 0 {
		s.unLockRetryDelayMS = DefaultUnLockRetryDelayMS
	} else {
		s.unLockRetryDelayMS = unLockRetryDelayMS
	}

}
func (s *lockFactory) GetUnLockRetryDelayMS() int64 {
	return s.unLockRetryDelayMS
}

//加锁
func (s *lockFactory) Lock(ctx context.Context, keyName string, lockMillSecond int) (lock *RLLock, err error) {

	conn := s.pool.Get()
	defer func() {
		err := conn.Close()
		if err != nil {
			fmt.Printf("关闭redis池失败: %s \n", err.Error())
		}
	}()
	lockTime := lockMillSecond

	randomData := GetRandomValue()
	retry := s.GetLockRetryCount()
	// 重试机制
	for retry >= 0 {
		select {
		// 如果处理完直接返回
		// Done:返回一个 Channel，这个 Channel 会在当前工作完成或者上下文被取消之后关闭，多次调用 Done 方法会返回同一个 Channel；
		case <-ctx.Done():
			fmt.Println("Lock-执行结束")
			return nil, ctx.Err()
		default:
			beginTime := time.Now().UnixNano()
			retry -= 1
			// 加锁
			luaRes, err := luaScriptSetnx.Do(conn, keyName, randomData, lockTime)
			if err != nil {
				fmt.Printf("redis分布式锁-加锁失败-时间: %v,key:%s, value:%s, expire: %d ms, err:%s \n", beginTime, keyName, randomData, lockMillSecond, err.Error())
				if retry != 0 {
					fmt.Println("加锁失败-准备重试加锁")
					time.Sleep(time.Duration(s.GetLockRetryDelayMS()) * time.Millisecond)
					continue
				}
			}
			res, err := redis.Int64(luaRes, err)
			if res == 1 {
				lock = new(RLLock)
				lock.ContinuedTime = lockTime
				lock.RandomKey = randomData
				lock.KeyName = keyName
				lock.CreateTime = time.Now().UnixNano() / 1e6
				//如果需要持续等任务执行完，那锁也应该让他保持一样的状态
				if s.isKeepAlive {
					lock.IsLive = true
					// 保证续命groutine能停止
					lock.RedisChan = make(chan struct{})
					go s.RenewalLife(lock, lockTime/2)
				}
				fmt.Println("加锁成功")
				return lock, nil
			} else {
				fmt.Printf("%s 加锁失败: %+v \n", keyName, res)
				if retry != 0 {
					time.Sleep(time.Duration(s.GetLockRetryDelayMS()) * time.Millisecond)
					continue
				}
				return nil, nil
			}

		}
	}

	return nil, errors.New("加锁失败")
}

//解锁
func (s *lockFactory) UnLock(ctx context.Context, lock *RLLock) (isUnLock bool, err error) {
	// 解锁
	if lock == nil {
		return false, errors.New("lock参数为空")
	}
	//mutex 锁
	mutexLock.Lock()
	defer mutexLock.Unlock()
	// 如果已经解锁了直接返回
	if lock.IsUnlock {
		fmt.Printf("%s:已经解锁了", lock.KeyName)
		return true, nil
	}

	if lock.IsLive {
		if !lock.ChanIsNoIn {
			//这里写入
			lock.RedisChan <- struct{}{}
			lock.ChanIsNoIn = true
		}
	} else {
		lock.UnlockTime = time.Now().UnixNano() / 1e6
		lock.IsUnlock = true
	}

	conn := s.pool.Get()
	defer func() {
		err := conn.Close()
		if err != nil {
			fmt.Printf("关闭redis池失败: %s \n", err.Error())
		}
	}()

	retry := s.GetUnLockRetryCount()

	for retry >= 0 {
		select {
		case <-ctx.Done():
			fmt.Println("UnLock-执行结束")
			return false, ctx.Err()
		default:
			retry -= 1
			// 删除对应的键---KeyName
			res, err := redis.Int64(luaScriptDelete.Do(conn, lock.KeyName, lock.RandomKey))
			if err != nil {
				fmt.Printf("%s 解锁失败，报错信息:%+v", lock.KeyName, err.Error())
				if retry != 0 {
					time.Sleep(time.Duration(s.GetUnLockRetryDelayMS()) * time.Millisecond)
					continue
				}
				return false, err
			}
			if res != 1 {
				fmt.Printf("%s 解锁失败，报错信息:%+v \n", lock.KeyName, res)
				return false, nil
			}
			fmt.Println("解锁成功")
			return true, nil
		}
	}
	return false, errors.New("解锁失败")
}

// 续命
func (s *lockFactory) RenewalLife(lock *RLLock, continuedTime int) {
	timer := time.NewTimer(time.Duration(continuedTime) * time.Millisecond)
	conn := s.pool.Get()
	if continuedTime < 0 {
		continuedTime = DefaultContinuedTime
	}
	defer func() {
		if !timer.Stop() {
			// <-timer.C失败直接走默认---往下接着走
			select {
			case <-timer.C:
			default:
			}
		}
		err := conn.Close()
		if err != nil {
			fmt.Printf("关闭redis池失败: %s \n", err.Error())

		}

	}()

	// todo 这里加个错误重试次数吧
	for {
		if lock.IsUnlock {
			return
		}
		timer.Reset(time.Duration(continuedTime) * time.Millisecond)
		select {
		// 这里读出，让消息关闭
		case <-lock.RedisChan:
			fmt.Print("keepAlive-closeChan \n")
			lock.IsUnlock = true
			lock.UnlockTime = time.Now().UnixNano() / 1e6
			close(lock.RedisChan)
			return
		case <-timer.C:
			// 默认 500毫秒进行续命一次
			fmt.Print("keepAlive-开始续命 \n")
			luaRes, err := luaScriptExpire.Do(conn, lock.KeyName, lock.KeyName, lock.RandomKey, lock.ContinuedTime)
			if err != nil {
				fmt.Printf("%s, 续命失败:%+v \n", lock.KeyName, err.Error())
				continue
			}
			res, err := redis.Int64(luaRes, err)
			if res != 1 {
				fmt.Printf("%s 续命失败: %+v", lock.KeyName, res)
				lock.IsUnlock = true
				lock.UnlockTime = time.Now().UnixNano() / 1e6
				close(lock.RedisChan)
				return
			}
			fmt.Println("续命成功")
			lock.ContinuedTimeMS = time.Now().UnixNano() / 1e6
		}
	}
}
