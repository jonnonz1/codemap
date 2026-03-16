package parse

// Registry maps file extensions to their Parser implementations.
type Registry struct {
	byExt map[string]Parser
}

// NewRegistry creates an empty parser registry.
func NewRegistry() *Registry {
	return &Registry{byExt: make(map[string]Parser)}
}

// Register adds a parser for all its declared extensions.
func (r *Registry) Register(p Parser) {
	for _, ext := range p.Extensions() {
		r.byExt[ext] = p
	}
}

// ForExtension returns the parser for the given file extension, or nil.
func (r *Registry) ForExtension(ext string) Parser {
	return r.byExt[ext]
}
