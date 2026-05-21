package core

// EntityRegistry manages all registered entities
type EntityRegistry struct {
	entities map[string]Entity
}

// NewEntityRegistry creates a new entity registry
func NewEntityRegistry() *EntityRegistry {
	return &EntityRegistry{
		entities: make(map[string]Entity),
	}
}

// Register registers a new entity
func (r *EntityRegistry) Register(entity Entity) {
	r.entities[entity.Name()] = entity
}

// Get retrieves an entity by name
func (r *EntityRegistry) Get(name string) (Entity, bool) {
	entity, exists := r.entities[name]
	return entity, exists
}

// List returns all registered entity names
func (r *EntityRegistry) List() []string {
	names := make([]string, 0, len(r.entities))
	for name := range r.entities {
		names = append(names, name)
	}
	return names
}
