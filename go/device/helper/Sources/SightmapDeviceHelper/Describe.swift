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
    let root = tree(response.elements.elements)
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
