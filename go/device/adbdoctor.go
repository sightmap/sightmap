package device

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// DoctorAndroid checks for everything `sightmap device --platform android`
// needs: adb, and a device to drive or an AVD to boot. Nothing is built.
func DoctorAndroid(ctx context.Context) []Check {
	path, err := FindADB()
	if err != nil {
		e := AsError(err)
		return []Check{fail("adb", e.Message, e.Hint)}
	}
	adb := ADB{Path: path}
	version, err := adb.run(ctx, 15*time.Second, "version")
	if err != nil {
		return []Check{fail("adb", path+": "+AsError(err).Message, "reinstall the SDK platform-tools")}
	}
	first, _, _ := strings.Cut(strings.TrimSpace(version), "\n")
	checks := []Check{pass("adb", fmt.Sprintf("%s (%s)", path, first))}
	checks = append(checks, checkAndroidDevices(ctx, adb, ListAVDs(ctx))...)
	return checks
}

func checkAndroidDevices(ctx context.Context, adb ADB, avds []string) []Check {
	devices, err := adb.ListAndroidDevices(ctx)
	if err != nil {
		return []Check{fail("devices", AsError(err).Message, "run `adb kill-server`, then `adb devices`")}
	}
	var online, other []string
	for _, d := range devices {
		if d.State == "device" {
			online = append(online, fmt.Sprintf("%s (%s)", d.Serial, d.Label()))
		} else {
			other = append(other, fmt.Sprintf("%s is %s", d.Serial, d.State))
		}
	}
	var checks []Check
	switch {
	case len(online) > 0:
		checks = append(checks, pass("devices", strings.Join(online, "; ")))
	case len(avds) > 0:
		checks = append(checks, pass("devices", "none connected; `start --device "+avds[0]+"` boots an AVD (available: "+strings.Join(avds, ", ")+")"))
	default:
		checks = append(checks, fail("devices", "no device connected and no AVD to boot",
			"create an emulator in Android Studio's Device Manager (or `avdmanager create avd`), or connect a device with USB debugging on"))
	}
	if len(other) > 0 {
		checks = append(checks, fail("device state", strings.Join(other, "; "), "accept the USB debugging prompt on the device, or wait for the emulator to finish booting"))
	}
	return checks
}
