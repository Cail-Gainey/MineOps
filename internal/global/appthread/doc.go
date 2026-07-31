// Package appthread 提供 MineOps 全应用共用的有界 goroutine 线程池。
//
// 它是唯一的「线程管理」边界:所有临时后台 goroutine 与并发扇出
// (例如下载源测速)都应经由 Pool 执行,使总并发保持有界、
// panic 被转换成结构化崩溃日志而不是让进程挂掉,
// 并且关闭时能够等待进行中的工作完成。
//
// 进程级 Pool 通过 Default 获取;需要确定性归属与便于测试时,
// 可用 NewPool 创建作用域内的 Pool 并注入。
package appthread
