package dev.sightmap.sample

import android.os.Bundle
import android.view.LayoutInflater
import android.view.View
import android.view.ViewGroup
import android.widget.TextView
import androidx.appcompat.app.AppCompatActivity
import androidx.core.widget.doAfterTextChanged
import androidx.recyclerview.widget.LinearLayoutManager
import androidx.recyclerview.widget.RecyclerView
import com.google.android.material.bottomnavigation.BottomNavigationView
import com.google.android.material.button.MaterialButtonToggleGroup
import com.google.android.material.dialog.MaterialAlertDialogBuilder
import com.google.android.material.textfield.TextInputEditText

class ViewsActivity : AppCompatActivity() {
    private var query = ""
    private var filter = 0
    private var inCart = 0
    private val adapter = ProductAdapter { p ->
        MaterialAlertDialogBuilder(this)
            .setTitle(p.title)
            .setMessage("${p.brand}, ${p.price}")
            .setPositiveButton("Add to cart") { _, _ -> inCart++; findViewById<TextView>(R.id.cart).text = "In cart: $inCart" }
            .setNegativeButton("Cancel", null)
            .show()
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(R.layout.activity_views)
        val list = findViewById<RecyclerView>(R.id.product_list)
        list.layoutManager = LinearLayoutManager(this)
        list.adapter = adapter
        refresh()
        findViewById<TextInputEditText>(R.id.search).doAfterTextChanged { query = it?.toString() ?: ""; refresh() }
        findViewById<MaterialButtonToggleGroup>(R.id.filter).addOnButtonCheckedListener { _, id, checked ->
            if (checked) {
                filter = when (id) { R.id.filter_sale -> 1; R.id.filter_new -> 2; else -> 0 }
                refresh()
            }
        }
        val nav = findViewById<BottomNavigationView>(R.id.bottom_nav)
        nav.selectedItemId = R.id.nav_views
        nav.setOnItemSelectedListener { item ->
            if (item.itemId == R.id.nav_compose) finish()
            true
        }
    }

    private fun refresh() = adapter.submit(products.shown(query, filter))
}

class ProductAdapter(private val onClick: (Product) -> Unit) : RecyclerView.Adapter<ProductAdapter.Holder>() {
    private var items = listOf<Product>()

    fun submit(list: List<Product>) {
        items = list
        notifyDataSetChanged()
    }

    class Holder(view: View) : RecyclerView.ViewHolder(view) {
        val title: TextView = view.findViewById(R.id.product_title)
        val brand: TextView = view.findViewById(R.id.product_brand)
        val price: TextView = view.findViewById(R.id.product_price)
    }

    override fun onCreateViewHolder(parent: ViewGroup, viewType: Int) =
        Holder(LayoutInflater.from(parent.context).inflate(R.layout.item_product, parent, false))

    override fun getItemCount() = items.size

    override fun onBindViewHolder(holder: Holder, position: Int) {
        val p = items[position]
        holder.title.text = p.title
        holder.brand.text = p.brand
        holder.price.text = p.price
        holder.itemView.setOnClickListener { onClick(p) }
    }
}
