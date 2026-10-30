import FBSimulatorAX
import FBSimulatorControl
import Foundation

enum Describe {
  static let keys: Set<AXKeys> = [.type, .role, .subrole, .traits, .label, .uniqueID, .value, .frameDict, .enabled]

  static let accessibilityDisabledMessage =
    "the app exposes no accessibility elements; enable app accessibility with "
    + "`xcrun simctl spawn %@ defaults write com.apple.Accessibility ApplicationAccessibilityEnabled -bool true` "
    + "and relaunch the app"

  static func run(_ connection: SimulatorConnection, udid: String) async throws -> [String: Any] {
    let options = AccessibilityRequestOptions(format: .nested, keys: keys)
    let response = try await connection.automation.describe(.frontmost, options: options)
    let root = await expandOpaqueBars(connection, tree(response.elements.elements))
    if hasChildren(root) {
      return ["screen": screen(connection.simulator, root: root), "root": root]
    }
    // A system sheet from another process (e.g. "Save Password?") leaves the
    // frontmost app's tree empty; describe the process that owns the screen.
    // Only the sheet's controls answer a hit test, not its background, so probe
    // points from the center outward until one lands on an element.
    let size = screen(connection.simulator, root: root)
    if let pid = await overlayOwner(connection, width: size["w"] as? Double ?? 0, height: size["h"] as? Double ?? 0),
      let overlay = try? await connection.automation.describe(.application(pid: pid), options: options)
    {
      let overlayRoot = tree(overlay.elements.elements)
      if hasChildren(overlayRoot) {
        return ["screen": size, "root": overlayRoot, "overlay": true]
      }
    }
    throw HelperError(code: "ax_disabled", message: String(format: accessibilityDisabledMessage, udid))
  }

  private static func overlayOwner(_ connection: SimulatorConnection, width: Double, height: Double) async -> pid_t? {
    let probe = AccessibilityRequestOptions(format: .nested, keys: [.pid])
    for fy in [0.5, 0.6, 0.4, 0.7, 0.3, 0.8, 0.2, 0.9] {
      for fx in [0.5, 0.25, 0.75] {
        let point = CGPoint(x: width * fx, y: height * fy)
        if let hit = try? await connection.automation.describe(.point(point), options: probe).elements.elements.first,
          let pid = hit.pid.flatMap({ $0 })
        {
          return pid_t(pid)
        }
      }
    }
    return nil
  }

  // Out of process, iOS 26 reports some bars (the floating tab bar) as one
  // childless element, yet their items answer a hit test. Probe a line of
  // points along each such bar and adopt the distinct elements hit inside it.
  private static func expandOpaqueBars(_ connection: SimulatorConnection, _ element: [String: Any]) async -> [String: Any] {
    var element = element
    if let children = element["children"] as? [[String: Any]], !children.isEmpty {
      var expanded: [[String: Any]] = []
      for child in children {
        expanded.append(await expandOpaqueBars(connection, child))
      }
      element["children"] = expanded
      return element
    }
    guard isOpaqueBar(element), let bar = rect(element["frame"]) else { return element }
    let options = AccessibilityRequestOptions(format: .nested, keys: keys)
    var items: [[String: Any]] = []
    var seen = Set<String>()
    for x in stride(from: bar.minX + barProbeStep / 2, to: bar.maxX, by: barProbeStep) {
      guard let hit = try? await connection.automation.describe(.point(CGPoint(x: x, y: bar.midY)), options: options).elements.elements.first
      else { continue }
      let item = node(hit)
      if let frame = rect(item["frame"]), frame != bar, bar.contains(CGPoint(x: frame.midX, y: frame.midY)), seen.insert("\(frame)").inserted {
        items.append(item)
      }
    }
    if !items.isEmpty {
      element["children"] = items
    }
    return element
  }

  private static let barProbeStep = 20.0
  private static let leafTraits: Set<String> = ["Button", "Link", "Adjustable", "StaticText", "Image", "SearchField", "KeyboardKey"]

  private static func isOpaqueBar(_ element: [String: Any]) -> Bool {
    guard let frame = rect(element["frame"]), frame.width >= 88, (44...120).contains(frame.height) else { return false }
    let traits = element["traits"] as? [String] ?? []
    return element["identifier"] is NSNull && element["value"] is NSNull && leafTraits.isDisjoint(with: traits)
  }

  private static func rect(_ frame: Any?) -> CGRect? {
    guard let f = frame as? [String: Any], let x = f["x"] as? CGFloat, let y = f["y"] as? CGFloat,
      let w = f["w"] as? CGFloat, let h = f["h"] as? CGFloat
    else { return nil }
    return CGRect(x: x, y: y, width: w, height: h)
  }

  private static func tree(_ elements: [AccessibilityDocumentElement]) -> [String: Any] {
    elements.count == 1 ? node(elements[0]) : syntheticRoot(children: elements.map(node))
  }

  private static func hasChildren(_ root: [String: Any]) -> Bool {
    !((root["children"] as? [Any])?.isEmpty ?? true)
  }

  private static func node(_ element: AccessibilityDocumentElement) -> [String: Any] {
    [
      "role": json(element.role),
      "subrole": json(element.subrole),
      "type": json(element.type),
      "label": json(element.label),
      "identifier": json(element.identifier),
      "value": json(element.value.flatMap { $0 }.flatMap(string)),
      "frame": frame(element.frame.flatMap { $0 }?.rect),
      "enabled": element.enabled.flatMap { $0 } ?? true,
      "traits": element.traits.flatMap { $0 } ?? [],
      "children": (element.children ?? []).map(node),
    ]
  }

  private static func syntheticRoot(children: [[String: Any]]) -> [String: Any] {
    [
      "role": "AXGroup", "subrole": NSNull(), "type": "Group", "traits": [String](), "label": NSNull(), "identifier": NSNull(), "value": NSNull(),
      "frame": frame(nil), "enabled": true, "children": children,
    ]
  }

  private static func screen(_ simulator: Simulator, root: [String: Any]) -> [String: Any] {
    if let info = simulator.screenInfo, info.scale > 0 {
      return [
        "w": Double(info.widthPixels) / Double(info.scale),
        "h": Double(info.heightPixels) / Double(info.scale),
      ]
    }
    let rootFrame = root["frame"] as? [String: Any]
    return ["w": rootFrame?["w"] ?? 0, "h": rootFrame?["h"] ?? 0]
  }

  private static func frame(_ rect: CGRect?) -> [String: Any] {
    let rect = rect ?? .zero
    return ["x": rect.origin.x, "y": rect.origin.y, "w": rect.size.width, "h": rect.size.height]
  }

  private static func json(_ value: String??) -> Any {
    value.flatMap { $0 }.map { $0 as Any } ?? NSNull()
  }

  private static func json(_ value: String?) -> Any {
    value.map { $0 as Any } ?? NSNull()
  }

  private static func string(_ value: AccessibilityAttributeValue) -> String? {
    switch value {
    case .string(let string): return string
    case .bool(let bool): return bool ? "1" : "0"
    case .int(let int): return String(int)
    case .double(let double): return String(double)
    case .null: return nil
    case .array, .object:
      guard let data = try? JSONEncoder().encode(value) else { return nil }
      return String(decoding: data, as: UTF8.self)
    @unknown default:
      return nil
    }
  }
}
