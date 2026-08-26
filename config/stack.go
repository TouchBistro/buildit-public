package config

// Stack for DFS
type stack[T comparable] struct {
	resources []T
}

// Push
func (s *stack[T]) Push(element T) {
	s.resources = append(s.resources, element)
}

// Pop
func (s *stack[T]) Pop() T {
	element := s.resources[len(s.resources)-1]
	s.resources = s.resources[:len(s.resources)-1]
	return element
}

// Top
func (s *stack[T]) Top() T {
	return s.resources[len(s.resources)-1]
}

// Find cycle using the stack. Only called if a cycle exists
func (s *stack[T]) FindCycle(element T) []T {
	cycle := []T{element}
	for s.Top() != element {
		next := s.Pop()
		cycle = append(cycle, next)
	}
	cycle = append(cycle, element)
	return cycle
}
