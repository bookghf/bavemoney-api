// Package category owns the two-level income/expense category tree.
package category

import (
	"errors"
	"regexp"
)

// Category is the API representation of a category. Children is populated only
// when the category is returned as part of a tree.
type Category struct {
	ID       string     `json:"id"`
	ParentID string     `json:"parent_id,omitempty"`
	Name     string     `json:"name"`
	Type     string     `json:"type"`
	Icon     string     `json:"icon,omitempty"`
	Color    string     `json:"color,omitempty"`
	IsSystem bool       `json:"is_system"`
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

// Colors are palette keys the app maps to light and dark shades.
var colors = map[string]bool{
	"orange": true, "amber": true, "lime": true, "cyan": true, "indigo": true,
	"violet": true, "fuchsia": true, "pink": true, "brown": true, "slate": true,
}

// iconPattern matches Ionicons glyph names such as "fast-food" or "game-controller".
var iconPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// checkLook validates an optional icon and color.
func checkLook(icon, color string) error {
	if icon != "" && (len(icon) > 40 || !iconPattern.MatchString(icon)) {
		return errors.New("icon must be an icon name such as fast-food")
	}
	if color != "" && !colors[color] {
		return errors.New("color must be one of orange, amber, lime, cyan, indigo, violet, fuchsia, pink, brown, slate")
	}
	return nil
}
