import SwiftUI
import os

let appLog = Logger(subsystem: "dev.sightmap.SimSample", category: "app")

final class Session: ObservableObject {
    @Published var signedInEmail: String?
}

@main
struct SampleApp: App {
    @StateObject private var session = Session()

    init() {
        appLog.info("launched")
        print("Sightmap Sim Sample launched")
    }

    var body: some Scene {
        WindowGroup {
            if let email = session.signedInEmail {
                MainTabs(email: email)
                    .environmentObject(session)
            } else {
                LoginView()
                    .environmentObject(session)
            }
        }
    }
}

struct LoginView: View {
    @EnvironmentObject private var session: Session
    @State private var email = ""
    @State private var password = ""
    @State private var error: String?

    var body: some View {
        VStack(spacing: 16) {
            Text("Sightmap Sim Sample")
                .font(.largeTitle.bold())
                .accessibilityIdentifier("login-title")
            TextField("Email", text: $email)
                .textContentType(.emailAddress)
                .keyboardType(.emailAddress)
                .textInputAutocapitalization(.never)
                .autocorrectionDisabled()
                .textFieldStyle(.roundedBorder)
                .accessibilityIdentifier("login-email")
            SecureField("Password", text: $password)
                .textFieldStyle(.roundedBorder)
                .accessibilityIdentifier("login-password")
            if let error {
                Text(error)
                    .foregroundStyle(.red)
                    .accessibilityIdentifier("login-error")
            }
            Button("Sign In", action: signIn)
                .buttonStyle(.borderedProminent)
                .accessibilityIdentifier("login-submit")
        }
        .padding(24)
    }

    private func signIn() {
        guard email.contains("@"), !password.isEmpty else {
            error = "Enter an email and password"
            appLog.error("sign-in rejected: missing email or password")
            print("sign-in rejected")
            return
        }
        appLog.info("signed in as \(email, privacy: .public)")
        print("signed in as \(email)")
        session.signedInEmail = email
    }
}

struct MainTabs: View {
    let email: String

    var body: some View {
        TabView {
            ProductList()
                .tabItem {
                    Label("Products", systemImage: "bag")
                        .accessibilityIdentifier("tab-products")
                }
            NetworkView()
                .tabItem {
                    Label("Network", systemImage: "network")
                        .accessibilityIdentifier("tab-network")
                }
            AccountView(email: email)
                .tabItem {
                    Label("Account", systemImage: "person.crop.circle")
                        .accessibilityIdentifier("tab-account")
                }
            CatalogList()
                .tabItem {
                    Label("Catalog", systemImage: "square.grid.2x2")
                        .accessibilityIdentifier("tab-catalog")
                }
        }
    }
}

struct Product: Identifiable, Hashable {
    let id: Int
    var name: String { "Product \(id)" }
    var price: String { String(format: "$%d.%02d", 5 + id % 95, (id * 37) % 100) }

    static let all = (1...200).map(Product.init)
}

struct ProductList: View {
    var body: some View {
        NavigationStack {
            List(Product.all) { product in
                NavigationLink(value: product) {
                    HStack {
                        Text(product.name)
                        Spacer()
                        Text(product.price).foregroundStyle(.secondary)
                    }
                }
                .accessibilityIdentifier("product-row-\(product.id)")
            }
            .navigationTitle("Products")
            .navigationDestination(for: Product.self) { ProductDetail(product: $0) }
        }
    }
}

// The catalog's rows are built the way list rows usually are: a NavigationLink
// whose label is a custom view of several Texts, which SwiftUI joins into one
// accessibility label. Titles and makers contain ", " on purpose, so the joined
// label cannot be split back apart.
struct CatalogItem: Identifiable, Hashable {
    struct Summary: Hashable {
        let id: Int
        let title: String
        let price: Double
    }

    let summary: Summary
    let maker: String
    var id: Int { summary.id }

    static let all = [
        CatalogItem(summary: Summary(id: 1, title: "Socks, Wool, Pair of 2", price: 9.99), maker: "Acme, Inc."),
        CatalogItem(summary: Summary(id: 2, title: "Desk Lamp", price: 24.5), maker: "Bright, Light & Co."),
        CatalogItem(summary: Summary(id: 3, title: "Mug, Large", price: 12), maker: "Kiln"),
    ]
}

struct CatalogList: View {
    var body: some View {
        NavigationStack {
            List(CatalogItem.all) { item in
                NavigationLink(value: item.id) {
                    CatalogRow(item: item.summary, maker: item.maker)
                }
                .accessibilityIdentifier("catalog-row-\(item.id)")
                .accessibilityCustomContent("Maker", item.maker)
                .accessibilityCustomContent("Price", item.summary.price.formatted(.currency(code: "USD")))
            }
            .navigationTitle("Catalog")
            .navigationDestination(for: Int.self) { id in Text("Item \(id)") }
        }
    }
}

struct CatalogRow: View {
    let item: CatalogItem.Summary
    let maker: String

    var body: some View {
        HStack(spacing: 12) {
            RoundedRectangle(cornerRadius: 8)
                .fill(.quaternary)
                .frame(width: 44, height: 44)
            VStack(alignment: .leading, spacing: 2) {
                Text(item.title)
                    .font(.headline)
                Text(maker)
                    .font(.subheadline)
                    .foregroundStyle(.secondary)
                Text(item.price, format: .currency(code: "USD"))
                    .font(.subheadline.monospacedDigit())
            }
        }
    }
}

struct ProductDetail: View {
    let product: Product
    @State private var inCart = false
    @State private var showOffer = false

    var body: some View {
        VStack(spacing: 16) {
            Text(product.name)
                .font(.title.bold())
                .accessibilityIdentifier("product-detail-name")
            Text(product.price)
                .font(.title2)
                .accessibilityIdentifier("product-detail-price")
            Button(inCart ? "Added" : "Add to Cart") {
                inCart = true
                appLog.info("added \(product.name, privacy: .public) to cart")
                print("added \(product.name) to cart")
            }
            .buttonStyle(.borderedProminent)
            .disabled(inCart)
            .accessibilityIdentifier("product-detail-add")
            Button("See Offer") { showOffer = true }
                .accessibilityIdentifier("product-detail-offer")
        }
        .padding()
        .navigationTitle(product.name)
        .navigationBarTitleDisplayMode(.inline)
        .fullScreenCover(isPresented: $showOffer) { OfferView(product: product) }
    }
}

// OfferView is presented full screen: UIKit drops touches until it has slid
// all the way up, while the accessibility tree already reports where it ends.
struct OfferView: View {
    let product: Product
    @Environment(\.dismiss) private var dismiss
    @State private var claimed = false

    var body: some View {
        VStack(spacing: 16) {
            Text("Offer on \(product.name)")
                .font(.title.bold())
                .accessibilityIdentifier("offer-title")
            Button(claimed ? "Claimed" : "Claim Offer") {
                claimed = true
                appLog.info("claimed the offer on \(product.name, privacy: .public)")
                print("claimed the offer on \(product.name)")
            }
            .buttonStyle(.borderedProminent)
            .accessibilityIdentifier("offer-claim")
            Button("Close") { dismiss() }
                .accessibilityIdentifier("offer-close")
        }
        .padding()
    }
}

struct NetworkView: View {
    @State private var status = "Idle"
    @State private var requestCount = 0

    var body: some View {
        VStack(spacing: 16) {
            Button("Send Request", action: send)
                .buttonStyle(.borderedProminent)
                .accessibilityIdentifier("network-send")
            Button("Fetch Report", action: fetch)
                .accessibilityIdentifier("network-fetch")
            Text(status)
                .accessibilityIdentifier("network-status")
        }
        .padding()
    }

    private func send() {
        requestCount += 1
        let path = "sightmap/order-\(requestCount)"
        // The networkBaseURL default points the request at a local server.
        let base = UserDefaults.standard.string(forKey: "networkBaseURL") ?? "https://httpbin.org/anything"
        var request = URLRequest(url: URL(string: "\(base)/\(path)")!)
        request.httpMethod = "POST"
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        request.setValue("Bearer sample-not-a-secret", forHTTPHeaderField: "Authorization")
        request.httpBody = try? JSONSerialization.data(withJSONObject: [
            "order": requestCount, "items": ["Product 1", "Product 2"], "sessionToken": "sample-not-a-secret",
        ])
        status = "Sending…"
        appLog.info("POST \(path, privacy: .public)")
        print("POST /anything/\(path)")
        URLSession.shared.dataTask(with: request) { _, response, error in
            let code = (response as? HTTPURLResponse)?.statusCode ?? -1
            let result = error.map { "failed: \($0.localizedDescription)" } ?? "HTTP \(code)"
            appLog.info("POST \(path, privacy: .public) -> \(result, privacy: .public)")
            print("POST /anything/\(path) -> \(result)")
            DispatchQueue.main.async { status = result }
        }.resume()
    }

    // fetch GETs every path in the networkFetchPaths default at once, each on
    // its own URLSession, so CFNetwork can log their HAR entries concurrently.
    private func fetch() {
        let base = UserDefaults.standard.string(forKey: "networkBaseURL") ?? "https://httpbin.org/anything"
        let paths = UserDefaults.standard.stringArray(forKey: "networkFetchPaths") ?? ["sightmap/report"]
        status = "Fetching…"
        let group = DispatchGroup()
        for path in paths {
            group.enter()
            let session = URLSession(configuration: .ephemeral)
            session.dataTask(with: URL(string: "\(base)/\(path)")!) { data, response, error in
                let code = (response as? HTTPURLResponse)?.statusCode ?? -1
                let result = error.map { "failed: \($0.localizedDescription)" } ?? "HTTP \(code), \(data?.count ?? 0) bytes"
                appLog.info("GET \(path, privacy: .public) -> \(result, privacy: .public)")
                print("GET /anything/\(path) -> \(result)")
                group.leave()
            }.resume()
            session.finishTasksAndInvalidate()
        }
        group.notify(queue: .main) { status = "Fetched \(paths.count)" }
    }
}

struct AccountView: View {
    @EnvironmentObject private var session: Session
    let email: String

    var body: some View {
        NavigationStack {
            VStack(spacing: 16) {
                Text(email)
                    .accessibilityIdentifier("account-email")
                NavigationLink("Controls") { ControlsView() }
                    .accessibilityIdentifier("account-controls")
                Button("Sign Out") {
                    appLog.info("signed out")
                    print("signed out")
                    session.signedInEmail = nil
                }
                .accessibilityIdentifier("account-sign-out")
            }
            .padding()
        }
    }
}

// ControlsView holds controls that need more than a tap at their center from
// outside the app: a segmented control that comes out as one element without
// its segments, and a Form toggle whose center is its label.
struct ControlsView: View {
    @State private var segment = "Day"
    @State private var notify = true

    var body: some View {
        Form {
            Picker("Range", selection: $segment) {
                ForEach(["Day", "Week", "Month"], id: \.self) { Text($0) }
            }
            .pickerStyle(.segmented)
            .accessibilityIdentifier("controls-range")
            Toggle("Notify", isOn: $notify)
                .accessibilityIdentifier("controls-notify")
                .onChange(of: notify) { _, on in
                    appLog.info("notify \(on ? "on" : "off", privacy: .public)")
                    print("notify \(on ? "on" : "off")")
                }
        }
        .navigationTitle("Controls")
        .toolbar {
            ToolbarItem(placement: .topBarTrailing) {
                Button("Edit") {}.accessibilityIdentifier("controls-edit")
            }
            ToolbarItemGroup(placement: .bottomBar) {
                Button("Share", systemImage: "square.and.arrow.up") {}
                    .accessibilityIdentifier("controls-share")
                Spacer()
                Button("Delete", systemImage: "trash") {}
                    .accessibilityIdentifier("controls-delete")
            }
        }
    }
}
