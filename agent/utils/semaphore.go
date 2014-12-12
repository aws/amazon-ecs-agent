package utils

// Implements a simple counting sempahore on top of channels

type empty struct{}
type Semaphore interface {
	Post()
	Wait()
}

// Implements semaphore
type ChanSemaphore struct {
	semaphore chan empty
	Count     int // Public for introspection; should not be written to
}

func NewSemaphore(count int) Semaphore {
	sem := make(chan empty, count)
	// Init to all resources available
	for i := 0; i < count; i++ {
		sem <- empty{}
	}
	return &ChanSemaphore{
		semaphore: sem,
		Count:     count,
	}
}

func (s *ChanSemaphore) Post() {
	s.semaphore <- empty{}
}

func (s *ChanSemaphore) Wait() {
	<-s.semaphore
}
