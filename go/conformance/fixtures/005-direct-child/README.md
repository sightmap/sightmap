# 005-direct-child

A `form.search` root with two children: `inp1` (an `input` direct child) and `div1` (a `div` that itself contains `inp2`, another `input`). The sightmap selector `form.search > input` uses the direct-child combinator `>`. This fixture verifies that inp1 matches (it is a direct child of form.search) while inp2 does not (it is a grandchild, one extra level deep inside div1). The `>` combinator must not be silently upgraded to the descendant combinator.
