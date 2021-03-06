// -*- Mode: Go; indent-tabs-mode: t -*-

/*
 * Copyright (C) 2016 Canonical Ltd
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License version 3 as
 * published by the Free Software Foundation.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 *
 */

package main

import (
	"fmt"
	"io"
	"path/filepath"
	"text/tabwriter"

	"github.com/jessevdk/go-flags"

	"github.com/snapcore/snapd/client"
	"github.com/snapcore/snapd/i18n"
	"github.com/snapcore/snapd/osutil"
	"github.com/snapcore/snapd/snap"
)

type infoCmd struct {
	Verbose    bool `long:"verbose"`
	Positional struct {
		Snaps []string `positional-arg-name:"<snap>" required:"1"`
	} `positional-args:"yes" required:"yes"`
}

var shortInfoHelp = i18n.G("show detailed information about a snap")
var longInfoHelp = i18n.G(`
The info command shows detailed information about a snap, be it by name or by path.`)

func init() {
	addCommand("info",
		shortInfoHelp,
		longInfoHelp,
		func() flags.Commander {
			return &infoCmd{}
		}, map[string]string{
			"verbose": i18n.G("Include a verbose list of a snap's notes (otherwise, summarise notes)"),
		}, nil)
}

func norm(path string) string {
	path = filepath.Clean(path)
	if osutil.IsDirectory(path) {
		path = path + "/"
	}

	return path
}

func maybePrintType(w io.Writer, t string) {
	// XXX: using literals here until we reshuffle snap & client properly
	// (and os->core rename happens, etc)
	switch t {
	case "", "app", "application":
		return
	case "os":
		t = "core"
	}

	fmt.Fprintf(w, "type:\t%s\n", t)
}

func tryDirect(w io.Writer, path string, verbose bool) bool {
	path = norm(path)

	snapf, err := snap.Open(path)
	if err != nil {
		return false
	}

	info, err := snap.ReadInfoFromSnapFile(snapf, nil)
	if err != nil {
		return false
	}
	fmt.Fprintf(w, "path:\t%q\n", path)
	fmt.Fprintf(w, "name:\t%s\n", info.Name())
	fmt.Fprintf(w, "summary:\t%q\n", info.Summary())

	var notes *Notes
	if verbose {
		fmt.Fprintln(w, "notes:\t")
		fmt.Fprintf(w, "  confinement:\t%s\n", info.Confinement)
		if info.Broken == "" {
			fmt.Fprintln(w, "  broken:\tfalse")
		} else {
			fmt.Fprintf(w, "  broken:\ttrue (%s)\n", info.Broken)
		}

	} else {
		notes = NotesFromInfo(info)
	}
	fmt.Fprintf(w, "version:\t%s %s\n", info.Version, notes)
	maybePrintType(w, string(info.Type))

	return true
}

func coalesce(snaps ...*client.Snap) *client.Snap {
	for _, s := range snaps {
		if s != nil {
			return s
		}
	}
	return nil
}

func (x *infoCmd) Execute([]string) error {
	cli := Client()

	w := tabwriter.NewWriter(Stdout, 2, 2, 1, ' ', 0)

	noneOK := true
	for i, snapName := range x.Positional.Snaps {
		if i > 0 {
			fmt.Fprintln(w, "---")
		}

		if tryDirect(w, snapName, x.Verbose) {
			noneOK = false
			continue
		}

		remote, _, _ := cli.FindOne(snapName)
		local, _, _ := cli.Snap(snapName)

		both := coalesce(local, remote)

		if both == nil {
			fmt.Fprintf(w, "argument:\t%q\nwarning:\t%s\n", snapName, i18n.G("not a valid snap"))
			continue
		}
		noneOK = false

		fmt.Fprintf(w, "name:\t%s\n", both.Name)
		fmt.Fprintf(w, "summary:\t%q\n", both.Summary)
		// TODO: have publisher; use publisher here,
		// and additionally print developer if publisher != developer
		fmt.Fprintf(w, "publisher:\t%s\n", both.Developer)
		maybePrintType(w, both.Type)
		if x.Verbose {
			fmt.Fprintln(w, "notes:\t")
			fmt.Fprintf(w, "  private:\t%t\n", both.Private)
			fmt.Fprintf(w, "  confinement:\t%s\n", both.Confinement)
		}

		if local != nil {
			var notes *Notes
			if x.Verbose {
				jailMode := local.Confinement == client.DevModeConfinement && !local.DevMode
				fmt.Fprintf(w, "  devmode:\t%t\n", local.DevMode)
				fmt.Fprintf(w, "  jailmode:\t%t\n", jailMode)
				fmt.Fprintf(w, "  trymode:\t%t\n", local.TryMode)
				fmt.Fprintf(w, "  enabled:\t%t\n", local.Status == client.StatusActive)
				if local.Broken == "" {
					fmt.Fprintf(w, "  broken:\t%t\n", false)
				} else {
					fmt.Fprintf(w, "  broken:\t%t (%s)\n", true, local.Broken)
				}
			} else {
				notes = NotesFromLocal(local)
			}

			fmt.Fprintf(w, "tracking:\t%s\n", local.Channel)
			fmt.Fprintf(w, "installed:\t%s\t(%s)\t%s\n", local.Version, local.Revision, notes)
		}

		if remote != nil && remote.Channels != nil {
			// \t\t\t so we get "installed" lined up with "channels"
			fmt.Fprintf(w, "channels:\t\t\t\n")
			for _, ch := range []string{"stable", "candidate", "beta", "edge"} {
				m := remote.Channels[ch]
				if m == nil {
					continue
				}
				fmt.Fprintf(w, "  %s:\t%s\t(%s)\t%s\n", ch, m.Version, m.Revision, NotesFromRef(m))
			}
		}
	}
	w.Flush()

	if noneOK {
		return fmt.Errorf(i18n.G("no valid snaps given"))
	}

	return nil
}
