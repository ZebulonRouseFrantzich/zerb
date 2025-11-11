package platform_test

import (
	"context"
	"fmt"
	"log"

	"github.com/ZebulonRouseFrantzich/zerb/internal/platform"
)

func ExampleDetector_Detect() {
	detector := platform.NewDetector()
	info, err := detector.Detect(context.Background())
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("OS: %s\n", info.OS)
	fmt.Printf("Architecture: %s\n", info.Arch)

	if distro := info.GetDistro(); distro != nil {
		fmt.Printf("Distribution: %s (%s family)\n", distro.ID, distro.Family)
	}
}

func ExampleInfo_IsDebianFamily() {
	info := &platform.Info{
		OS:     "linux",
		Family: platform.FamilyDebian,
	}

	if info.IsDebianFamily() {
		fmt.Println("This is a Debian-based distribution")
	}
	// Output: This is a Debian-based distribution
}

func ExampleInfo_IsAppleSilicon() {
	// Example for Apple Silicon Mac
	info := &platform.Info{
		OS:   "darwin",
		Arch: "arm64",
	}

	if info.IsAppleSilicon() {
		fmt.Println("Running on Apple Silicon")
	}
	// Output: Running on Apple Silicon
}

func ExampleInfo_GetDistro() {
	// Example for Linux with distro information
	info := &platform.Info{
		OS:       "linux",
		Platform: "ubuntu",
		Family:   platform.FamilyDebian,
		Version:  "22.04",
	}

	if distro := info.GetDistro(); distro != nil {
		fmt.Printf("Distribution: %s %s (%s family)\n",
			distro.ID, distro.Version, distro.Family)
	}
	// Output: Distribution: ubuntu 22.04 (debian family)
}

func ExampleInfo_GetDistro_nil() {
	// Example for macOS (no distro information)
	info := &platform.Info{
		OS:   "darwin",
		Arch: "arm64",
	}

	if distro := info.GetDistro(); distro == nil {
		fmt.Println("No distribution information available (not Linux)")
	}
	// Output: No distribution information available (not Linux)
}
