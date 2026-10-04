// Package queue 提供通用的先进先出队列。
package queue

// Queue 是先进先出的集合。
type Queue[T any] []T

// Enqueue 将值追加到队列尾部。
func (q *Queue[T]) Enqueue(values ...T) {
	*q = append(*q, values...)
}

// Dequeue 移除并返回队列头部的值。
func (q *Queue[T]) Dequeue() T {
	value := (*q)[0]
	var zero T
	(*q)[0] = zero
	*q = (*q)[1:]
	return value
}

// Empty 返回队列是否为空。
func (q Queue[T]) Empty() bool {
	return len(q) == 0
}
