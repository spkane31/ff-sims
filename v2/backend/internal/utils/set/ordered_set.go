package set

type OrderedSet[T comparable] struct {
	elements []T
	index    map[T]struct{}
}

func NewOrderedSet[T comparable]() *OrderedSet[T] {
	return &OrderedSet[T]{
		elements: []T{},
		index:    make(map[T]struct{}),
	}
}

func (s *OrderedSet[T]) Add(element T) {
	if _, exists := s.index[element]; !exists {
		s.elements = append(s.elements, element)
		s.index[element] = struct{}{}
	}
}

func (s *OrderedSet[T]) Remove(element T) {
	if _, exists := s.index[element]; exists {
		delete(s.index, element)
		for i, el := range s.elements {
			if el == element {
				s.elements = append(s.elements[:i], s.elements[i+1:]...)
				break
			}
		}
	}
}

func (s *OrderedSet[T]) Contains(element T) bool {
	_, exists := s.index[element]
	return exists
}

func (s *OrderedSet[T]) Elements() []T {
	return s.elements
}

func (s *OrderedSet[T]) Size() int {
	return len(s.elements)
}
