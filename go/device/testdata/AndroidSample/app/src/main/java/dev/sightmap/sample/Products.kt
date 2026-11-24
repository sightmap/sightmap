package dev.sightmap.sample

data class Product(val id: Int, val title: String, val brand: String, val price: String, val sale: Boolean, val new: Boolean)

val products = listOf(
    Product(1, "Lash Mascara", "Essence", "$9.99", sale = true, new = false),
    Product(2, "Eyeshadow Palette", "Glamour", "$19.99", sale = false, new = true),
    Product(3, "Powder Canister", "Velvet", "$14.99", sale = false, new = false),
    Product(4, "Red Lipstick", "Chic", "$12.99", sale = true, new = false),
    Product(5, "Nail Polish", "Nail Couture", "$8.99", sale = false, new = true),
    Product(6, "Calvin Perfume", "Calvin", "$49.99", sale = false, new = false),
    Product(7, "Eau de Parfum", "Chanel", "$129.99", sale = true, new = false),
    Product(8, "Body Lotion", "Dior", "$89.99", sale = false, new = true),
    Product(9, "Gucci Bloom", "Gucci", "$79.99", sale = false, new = false),
    Product(10, "Bed Frame", "Annibale", "$1899.99", sale = true, new = false),
    Product(11, "Sofa", "Furniture Co", "$499.99", sale = false, new = true),
    Product(12, "Bedside Table", "Furniture Co", "$59.99", sale = false, new = false),
)

fun List<Product>.shown(query: String, filter: Int) =
    filter { (filter != 1 || it.sale) && (filter != 2 || it.new) && it.title.contains(query, ignoreCase = true) }
