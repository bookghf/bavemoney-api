-- Default categories every user starts with. System categories are global
-- (user_id NULL, is_system TRUE): they appear in every user's category list
-- but only admins can change them. Safe to re-run: rows that already exist
-- are skipped.

INSERT INTO categories (name, type, icon, is_system)
SELECT v.name, v.type, v.icon, TRUE
FROM (VALUES
    ('Salary',        'income',  'briefcase'),
    ('Bonus',         'income',  'gift'),
    ('Freelance',     'income',  'laptop'),
    ('Interest',      'income',  'trending-up'),
    ('Food',          'expense', 'fast-food'),
    ('Transport',     'expense', 'car'),
    ('Utilities',     'expense', 'flash'),
    ('Entertainment', 'expense', 'game-controller')
) AS v(name, type, icon)
WHERE NOT EXISTS (
    SELECT 1 FROM categories c
    WHERE c.is_system AND c.user_id IS NULL AND c.parent_id IS NULL
      AND c.name = v.name AND c.type = v.type
);

INSERT INTO categories (parent_id, name, type, is_system)
SELECT p.id, v.name, p.type, TRUE
FROM (VALUES
    ('Food',          'Groceries'),
    ('Food',          'Dining Out'),
    ('Transport',     'Fuel'),
    ('Transport',     'Parking'),
    ('Transport',     'Public Transport'),
    ('Utilities',     'Electricity'),
    ('Utilities',     'Water'),
    ('Utilities',     'Internet'),
    ('Entertainment', 'Movies'),
    ('Entertainment', 'Games')
) AS v(parent, name)
JOIN categories p
  ON p.is_system AND p.user_id IS NULL AND p.parent_id IS NULL
 AND p.type = 'expense' AND p.name = v.parent
WHERE NOT EXISTS (
    SELECT 1 FROM categories c
    WHERE c.is_system AND c.parent_id = p.id AND c.name = v.name
);
