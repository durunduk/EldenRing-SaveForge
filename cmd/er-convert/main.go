// Command er-convert converts an Elden Ring save between the native PS4
// container (decrypted memory.dat) and the native PC container (ER0000.sl2).
//
// It uses the same backend/core writer as the GUI's "Convert Format" tool, so
// the result is identical: BND4 header rebuild, per-entry MD5, Steam ID written
// to UserData10 and every active slot, DLC entry flag cleared on cross-platform
// writes. The source file is never modified.
//
// Usage:
//
//	er-convert -in memory.dat -out ER0000.sl2 -steamid 7656119xxxxxxxxxx
//	er-convert -in ER0000.sl2 -out memory.dat
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/oisis/EldenRing-SaveForge/backend/core"
)

func main() {
	in := flag.String("in", "", "source save (memory.dat or ER0000.sl2)")
	out := flag.String("out", "", "destination file (must not exist unless -force)")
	steam := flag.String("steamid", "", "SteamID64 (decimal) — required for PS4 → PC")
	force := flag.Bool("force", false, "overwrite -out if it exists")
	flag.Parse()

	if err := run(*in, *out, *steam, *force); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(in, out, steam string, force bool) error {
	if in == "" || out == "" {
		flag.Usage()
		return fmt.Errorf("-in and -out are required")
	}
	if absIn, _ := filepath.Abs(in); absIn != "" {
		if absOut, _ := filepath.Abs(out); absOut == absIn {
			return fmt.Errorf("-out must differ from -in")
		}
	}
	if _, err := os.Stat(out); err == nil && !force {
		return fmt.Errorf("%s already exists (use -force to overwrite)", out)
	}

	save, err := core.LoadSave(in)
	if err != nil {
		return fmt.Errorf("load %s: %w", in, err)
	}
	src := save.Platform
	fmt.Printf("source: %s (%s)\n", in, src)
	printSlots(save)

	switch src {
	case core.PlatformPS:
		if steam == "" {
			return fmt.Errorf("-steamid is required for PS4 → PC (your SteamID64, the folder name in %%APPDATA%%\\EldenRing\\)")
		}
		id, err := strconv.ParseUint(steam, 10, 64)
		if err != nil || id == 0 {
			return fmt.Errorf("invalid -steamid %q", steam)
		}
		save.Platform = core.PlatformPC
		save.Encrypted = false
		save.IV = nil
		save.SteamID = id
	case core.PlatformPC:
		save.Platform = core.PlatformPS
		save.Encrypted = false
	default:
		return fmt.Errorf("unknown source platform %q", src)
	}

	if err := save.SaveFile(out); err != nil {
		return fmt.Errorf("write %s: %w", out, err)
	}

	// Re-open the result with the strict loader and compare every active slot.
	check, err := core.LoadSave(out)
	if err != nil {
		return fmt.Errorf("written file does not reload: %w", err)
	}
	if check.Platform != save.Platform {
		return fmt.Errorf("written file detected as %s, expected %s", check.Platform, save.Platform)
	}
	for i := 0; i < 10; i++ {
		if save.ActiveSlots[i] != check.ActiveSlots[i] {
			return fmt.Errorf("slot %d active flag changed after conversion", i)
		}
		if !save.ActiveSlots[i] {
			continue
		}
		a, b := save.Slots[i].Player, check.Slots[i].Player
		if core.UTF16ToString(a.CharacterName[:]) != core.UTF16ToString(b.CharacterName[:]) || a.Level != b.Level {
			return fmt.Errorf("slot %d name/level mismatch after reload", i)
		}
	}
	if check.Platform == core.PlatformPC {
		if check.SteamID != save.SteamID {
			return fmt.Errorf("SteamID mismatch after reload: %d", check.SteamID)
		}
		if err := check.ValidateSteamIDConsistency(); err != nil {
			return fmt.Errorf("SteamID consistency: %w", err)
		}
	}

	fmt.Printf("written: %s (%s) — reload check OK\n", out, check.Platform)
	return nil
}

func printSlots(s *core.SaveFile) {
	for i := 0; i < 10; i++ {
		if !s.ActiveSlots[i] {
			continue
		}
		p := s.Slots[i].Player
		fmt.Printf("  slot %d: %q level %d\n", i, core.UTF16ToString(p.CharacterName[:]), p.Level)
	}
}
