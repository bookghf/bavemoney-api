// Package category owns the two-level income/expense category tree.
package category

// Category is the API representation of a category. Children is populated only
// when the category is returned as part of a tree.
type Category struct {
	ID       string     `json:"id"`
	ParentID string     `json:"parent_id,omitempty"`
	Name     string     `json:"name"`
	Type     string     `json:"type"`
	Icon     string     `json:"icon,omitempty"`
	Color    string     `json:"color,omitempty"`
	Children []Category `json:"children,omitempty"`
}

// CreateRequest is the POST /categories payload.
type CreateRequest struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	ParentID string `json:"parent_id,omitempty"`
	Icon     string `json:"icon,omitempty"`
	Color    string `json:"color,omitempty"`
}

// UpdateRequest is the PATCH /categories/{id} payload; nil fields are left alone.
type UpdateRequest struct {
	Name     *string `json:"name"`
	Type     *string `json:"type"`
	ParentID *string `json:"parent_id,omitempty"`
	Icon     *string `json:"icon,omitempty"`
	Color    *string `json:"color,omitempty"`
}

// Empty reports whether the request carries no field to update.
func (r UpdateRequest) Empty() bool {
	return r.Name == nil && r.Type == nil && r.ParentID == nil && r.Icon == nil && r.Color == nil
}

// BuildTree nests the flat category list under its top-level parents, keeping
// the input order within each level. Categories whose parent is missing from
// the list are omitted.
func BuildTree(flat []Category) []Category {
	type node struct {
		category Category
		children []*node
	}

	nodes := make(map[string]*node, len(flat))
	order := make([]*node, 0, len(flat))
	for _, category := range flat {
		category.Children = nil
		n := &node{category: category}
		nodes[category.ID] = n
		order = append(order, n)
	}

	for _, n := range order {
		if n.category.ParentID == "" {
			continue
		}
		if parent, ok := nodes[n.category.ParentID]; ok {
			parent.children = append(parent.children, n)
		}
	}

	var materialize func(*node) Category
	materialize = func(n *node) Category {
		category := n.category
		for _, child := range n.children {
			category.Children = append(category.Children, materialize(child))
		}
		return category
	}

	roots := []Category{}
	for _, n := range order {
		if n.category.ParentID == "" {
			roots = append(roots, materialize(n))
		}
	}
	return roots
}
