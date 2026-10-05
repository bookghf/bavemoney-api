-- More built-in categories (new users had only 4 expense categories), plus an
-- icon and a color key for every built-in. Colors are palette keys
-- ("orange", "violet", ...) the app maps to light/dark shades.

WITH wanted(name, type, icon, color) AS (VALUES
    ('Food',              'expense', 'fast-food',                 'orange'),
    ('Transport',         'expense', 'car',                       'indigo'),
    ('Utilities',         'expense', 'flash',                     'amber'),
    ('Entertainment',     'expense', 'game-controller',           'violet'),
    ('Shopping',          'expense', 'bag-handle',                'pink'),
    ('Household',         'expense', 'basket',                    'brown'),
    ('Clothing',          'expense', 'shirt',                     'fuchsia'),
    ('Health',            'expense', 'medkit',                    'cyan'),
    ('Personal Care',     'expense', 'sparkles',                  'lime'),
    ('Housing',           'expense', 'key',                       'slate'),
    ('Education',         'expense', 'school',                    'indigo'),
    ('Travel',            'expense', 'airplane',                  'cyan'),
    ('Gifts & Donations', 'expense', 'gift',                      'pink'),
    ('Lottery',           'expense', 'ticket',                    'amber'),
    ('Other',             'expense', 'ellipsis-horizontal-circle', 'slate'),
    ('Salary',            'income',  'briefcase',                 'lime'),
    ('Freelance',         'income',  'laptop',                    'cyan'),
    ('Bonus',             'income',  'gift',                      'amber'),
    ('Interest',          'income',  'trending-up',               'indigo'),
    ('Other Income',      'income',  'cash',                      'slate')
),
updated AS (
    UPDATE categories c SET icon = w.icon, color = w.color
    FROM wanted w
    WHERE c.is_system AND c.user_id IS NULL AND c.parent_id IS NULL
      AND lower(c.name) = lower(w.name) AND c.type = w.type
    RETURNING c.name, c.type
)
INSERT INTO categories (name, type, icon, color, is_system)
SELECT w.name, w.type, w.icon, w.color, TRUE
FROM wanted w
WHERE NOT EXISTS (
    SELECT 1 FROM categories c
    WHERE c.is_system AND c.user_id IS NULL AND c.parent_id IS NULL
      AND lower(c.name) = lower(w.name) AND c.type = w.type
);

-- A user's own top-level category with the same name and type as a built-in
-- would now show twice. Merge it into the built-in: its transactions,
-- budgets, and subcategories move over, then the duplicate goes.
CREATE TEMP TABLE category_merges ON COMMIT DROP AS
SELECT c.id AS custom_id, s.id AS system_id
FROM categories c
JOIN categories s
  ON s.is_system AND s.user_id IS NULL AND s.parent_id IS NULL
 AND lower(s.name) = lower(c.name) AND s.type = c.type
WHERE NOT c.is_system AND c.user_id IS NOT NULL AND c.parent_id IS NULL;

UPDATE transactions t SET category_id = m.system_id
FROM category_merges m WHERE t.category_id = m.custom_id;

UPDATE budgets b SET category_id = m.system_id
FROM category_merges m WHERE b.category_id = m.custom_id;

UPDATE categories c SET parent_id = m.system_id
FROM category_merges m WHERE c.parent_id = m.custom_id;

DELETE FROM categories c USING category_merges m WHERE c.id = m.custom_id;
