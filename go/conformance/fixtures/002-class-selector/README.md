# 002-class-selector

A tree of four nested `div` nodes: root → pc1 (class `product-card`) → pc2 (no class) → pc3 (classes `product-card featured`). The sightmap targets `div.product-card`. This fixture verifies that the class selector matches any node whose `classes` list includes the required class (pc1 and pc3 both match), while a plain `div` with no classes at all (pc2) is correctly excluded. It also confirms that having multiple classes does not prevent matching — pc3 matches because it contains `product-card` even though it also has `featured`.
