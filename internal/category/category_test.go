package category

import "testing"

func TestBuildTreeNestsChildrenUnderParents(t *testing.T) {
	roots := BuildTree([]Category{
		{ID: "food", Name: "Food", Type: "expense"},
		{ID: "groceries", ParentID: "food", Name: "Groceries", Type: "expense"},
		{ID: "salary", Name: "Salary", Type: "income"},
		{ID: "dining", ParentID: "food", Name: "Dining", Type: "expense"},
	})

	if len(roots) != 2 {
		t.Fatalf("got %d roots, want 2", len(roots))
	}
	if roots[0].ID != "food" || roots[1].ID != "salary" {
		t.Fatalf("roots = %s, %s; want food, salary", roots[0].ID, roots[1].ID)
	}
	if len(roots[0].Children) != 2 {
		t.Fatalf("food has %d children, want 2", len(roots[0].Children))
	}
	if roots[0].Children[0].ID != "groceries" || roots[0].Children[1].ID != "dining" {
		t.Errorf("children = %s, %s; want groceries, dining", roots[0].Children[0].ID, roots[0].Children[1].ID)
	}
	if len(roots[1].Children) != 0 {
		t.Errorf("salary has %d children, want 0", len(roots[1].Children))
	}
}

// A child listed before its parent must still be attached, since the flat list
// is ordered by name rather than by depth.
func TestBuildTreeHandlesChildBeforeParent(t *testing.T) {
	roots := BuildTree([]Category{
		{ID: "dining", ParentID: "food", Name: "Dining"},
		{ID: "food", Name: "Food"},
	})

	if len(roots) != 1 || roots[0].ID != "food" {
		t.Fatalf("roots = %+v, want a single food root", roots)
	}
	if len(roots[0].Children) != 1 || roots[0].Children[0].ID != "dining" {
		t.Errorf("children = %+v, want [dining]", roots[0].Children)
	}
}

func TestBuildTreeEmpty(t *testing.T) {
	if roots := BuildTree(nil); len(roots) != 0 {
		t.Errorf("BuildTree(nil) = %+v, want empty", roots)
	}
}
