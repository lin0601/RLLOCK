# 解决的问题
go在使用redis(setnx)分布锁的时候，我们业务处理的代码能否在这个过期时间内执行完代码。在原来的锁快过期的时候给他时间续上

# 设计思路
在加完锁后开启一个goroutine, 每隔一段时间进行延长锁的过期时间，判断是否业务处理完，根据context对象的生命周期

# 实现原理
## 1-使用了context
##### Deadline：方法需要返回当前 Context 被取消的时间，也就是完成工作的截止日期；
##### Done： 方法需要返回一个 Channel，这个 Channel 会在当前工作完成或者上下文被取消之后关闭，多次调用 Done 方法会返回同一个 Channel；
##### Err： 方法会返回当前 Context 结束的原因，它只会在 Done 返回的 Channel 被关闭时才会返回非空的值；如果当前 Context 被取消就会返回 Canceled 错误；如果当前 Context 超时就会返回 DeadlineExceeded 错误；
##### Value： 方法会从 Context 中返回键对应的值，对于同一个上下文来说，多次调用 Value 并传入相同的 Key 会返回相同的结果，这个功能可以用来传递请求特定的数据；
**学习链接**：https://www.bookstack.cn/read/draveness-golang/6ee8389651c21ac5.md#%E6%8E%A5%E5%8F%A3
## 2- lua命令， redigo工具类等等


# RLLock
