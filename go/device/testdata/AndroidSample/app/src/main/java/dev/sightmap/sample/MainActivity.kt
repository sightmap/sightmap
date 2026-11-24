package dev.sightmap.sample

import android.content.Intent
import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.selection.toggleable
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Home
import androidx.compose.material.icons.filled.Info
import androidx.compose.material.icons.filled.Search
import androidx.compose.material.icons.filled.Settings
import androidx.compose.material.icons.filled.Star
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.Checkbox
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.NavigationBar
import androidx.compose.material3.NavigationBarItem
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.PrimaryTabRow
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SegmentedButton
import androidx.compose.material3.SegmentedButtonDefaults
import androidx.compose.material3.SingleChoiceSegmentedButtonRow
import androidx.compose.material3.Switch
import androidx.compose.material3.Tab
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.ExperimentalComposeUiApi
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.semantics.testTagsAsResourceId
import androidx.compose.ui.unit.dp

class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContent { MaterialTheme { App() } }
    }
}

// Compose publishes a testTag as the node's resource-id only below a node that
// sets testTagsAsResourceId. The Shop and Settings tabs set it; the Plain tab
// shows the same Shop screen without it.
@OptIn(ExperimentalComposeUiApi::class)
fun Modifier.tagsAsIds() = semantics { testTagsAsResourceId = true }

@Composable
fun App() {
    var tab by rememberSaveable { mutableStateOf("shop") }
    var inCart by rememberSaveable { mutableIntStateOf(0) }
    val context = LocalContext.current
    val tabs = listOf(
        Triple("shop", "Shop", Icons.Filled.Home),
        Triple("plain", "Plain", Icons.Filled.Star),
        Triple("settings", "Settings", Icons.Filled.Settings),
        Triple("views", "Views", Icons.Filled.Info),
    )
    Scaffold(
        bottomBar = {
            NavigationBar(Modifier.tagsAsIds().testTag("nav")) {
                tabs.forEach { (key, label, icon) ->
                    NavigationBarItem(
                        selected = tab == key,
                        onClick = {
                            if (key == "views") context.startActivity(Intent(context, ViewsActivity::class.java)) else tab = key
                        },
                        icon = { Icon(icon, contentDescription = null) },
                        label = { Text(label) },
                        modifier = Modifier.testTag("nav.$key"),
                    )
                }
            }
        },
    ) { padding ->
        Box(Modifier.padding(padding).fillMaxSize()) {
            when (tab) {
                "shop" -> Box(Modifier.tagsAsIds()) { Shop("Shop", inCart) { inCart++ } }
                "plain" -> Shop("Shop (plain)", inCart) { inCart++ }
                "settings" -> Box(Modifier.tagsAsIds()) { Settings() }
            }
        }
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun Shop(title: String, inCart: Int, onAdd: () -> Unit) {
    var query by rememberSaveable { mutableStateOf("") }
    var filter by rememberSaveable { mutableIntStateOf(0) }
    var open by remember { mutableStateOf<Product?>(null) }
    Column {
        Row(Modifier.padding(16.dp), verticalAlignment = Alignment.CenterVertically) {
            Text(title, style = MaterialTheme.typography.headlineMedium, modifier = Modifier.semantics { heading() })
            Spacer(Modifier.weight(1f))
            Text("In cart: $inCart", modifier = Modifier.testTag("shop.cart"))
        }
        OutlinedTextField(
            value = query,
            onValueChange = { query = it },
            label = { Text("Search products") },
            leadingIcon = { Icon(Icons.Filled.Search, contentDescription = null) },
            singleLine = true,
            modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp).testTag("shop.search"),
        )
        val filters = listOf("All", "Sale", "New")
        SingleChoiceSegmentedButtonRow(Modifier.padding(16.dp).testTag("shop.filter")) {
            filters.forEachIndexed { i, label ->
                SegmentedButton(
                    selected = filter == i,
                    onClick = { filter = i },
                    shape = SegmentedButtonDefaults.itemShape(i, filters.size),
                    modifier = Modifier.testTag("shop.filter.${label.lowercase()}"),
                ) { Text(label) }
            }
        }
        LazyColumn(Modifier.testTag("shop.list")) {
            items(products.shown(query, filter), key = { it.id }) { p ->
                Row(
                    Modifier.fillMaxWidth().clickable { open = p }.padding(16.dp).testTag("product.${p.id}"),
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    Column(Modifier.weight(1f)) {
                        Text(p.title, style = MaterialTheme.typography.titleMedium, modifier = Modifier.testTag("product.title"))
                        Text(p.brand, modifier = Modifier.testTag("product.brand"))
                    }
                    Text(p.price, modifier = Modifier.testTag("product.price"))
                }
                HorizontalDivider()
            }
        }
    }
    open?.let { p ->
        ModalBottomSheet(onDismissRequest = { open = null }, modifier = Modifier.testTag("product.sheet")) {
            Column(Modifier.padding(24.dp)) {
                Text(p.title, style = MaterialTheme.typography.titleLarge, modifier = Modifier.testTag("product.sheet.title"))
                Text("${p.brand}, ${p.price}")
                Button(onClick = { onAdd(); open = null }, modifier = Modifier.padding(top = 16.dp).testTag("product.add")) {
                    Text("Add to cart")
                }
            }
        }
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun Settings() {
    var section by rememberSaveable { mutableIntStateOf(0) }
    var notifications by rememberSaveable { mutableStateOf(false) }
    var dark by rememberSaveable { mutableStateOf(false) }
    var marketing by rememberSaveable { mutableStateOf(false) }
    var confirm by remember { mutableStateOf(false) }
    var signedOut by rememberSaveable { mutableStateOf(false) }
    Column {
        PrimaryTabRow(selectedTabIndex = section, modifier = Modifier.testTag("settings.tabs")) {
            listOf("General", "Privacy").forEachIndexed { i, label ->
                Tab(selected = section == i, onClick = { section = i }, text = { Text(label) }, modifier = Modifier.testTag("settings.tab.${label.lowercase()}"))
            }
        }
        // A whole-row toggle: the row is the switch, and the Switch draws it.
        Row(
            Modifier.fillMaxWidth().toggleable(value = notifications, role = Role.Switch, onValueChange = { notifications = it })
                .padding(16.dp).testTag("settings.notifications"),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Text("Notifications", Modifier.weight(1f))
            Switch(checked = notifications, onCheckedChange = null)
        }
        // A label beside a Switch that takes its own taps.
        Row(Modifier.fillMaxWidth().padding(16.dp), verticalAlignment = Alignment.CenterVertically) {
            Text("Dark mode", Modifier.weight(1f))
            Switch(checked = dark, onCheckedChange = { dark = it }, modifier = Modifier.testTag("settings.dark"))
        }
        Row(
            Modifier.fillMaxWidth().toggleable(value = marketing, role = Role.Checkbox, onValueChange = { marketing = it })
                .padding(16.dp).testTag("settings.marketing"),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Checkbox(checked = marketing, onCheckedChange = null)
            Spacer(Modifier.width(8.dp))
            Text("Marketing email")
        }
        Text(if (signedOut) "Signed out" else "Signed in", Modifier.padding(16.dp).testTag("settings.status"))
        Button(onClick = { confirm = true }, modifier = Modifier.padding(16.dp).testTag("settings.signout")) { Text("Sign out") }
    }
    if (confirm) {
        AlertDialog(
            onDismissRequest = { confirm = false },
            title = { Text("Sign out?") },
            text = { Text("You can sign in again at any time.") },
            confirmButton = { TextButton(onClick = { signedOut = true; confirm = false }, modifier = Modifier.testTag("signout.confirm")) { Text("Sign out") } },
            dismissButton = { TextButton(onClick = { confirm = false }, modifier = Modifier.testTag("signout.cancel")) { Text("Cancel") } },
            modifier = Modifier.tagsAsIds().testTag("signout.dialog"),
        )
    }
}
