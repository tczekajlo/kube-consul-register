package services

// FactoryAdapter has a method to work with Controller resources.
type FactoryAdapter interface {
	Watch()
	Sync() error
	Clean() error
}
