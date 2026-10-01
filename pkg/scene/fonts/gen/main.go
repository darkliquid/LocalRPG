// Command gen downloads the OFL typefaces the video compositor embeds.
//
// Run it with `go generate ./pkg/scene/fonts`. The files land in
// pkg/scene/fonts/fonts/ and are committed, so a build never needs the network;
// if the download is unavailable the fonts package falls back to the Go fonts and
// everything still builds.
//
// The fonts are licensed under the SIL Open Font License, Version 1.1. Each
// family's notice is downloaded beside its .ttf as OFL-<Family>.txt and embedded
// with the binary. The licences are reproduced here so the terms travel with the
// downloader that fetches the fonts.
//
// --- EB Garamond -------------------------------------------------------------
// Copyright 2017 The EB Garamond Project Authors
// (https://github.com/octaviopardo/EBGaramond12)
//
// --- Inter -------------------------------------------------------------------
// Copyright 2020 The Inter Project Authors (https://github.com/rsms/inter)
//
// --- JetBrains Mono ----------------------------------------------------------
// Copyright 2020 The JetBrains Mono Project Authors
// (https://github.com/JetBrains/JetBrainsMono)
//
// This Font Software is licensed under the SIL Open Font License, Version 1.1.
// This license is copied below, and is also available with a FAQ at:
// https://openfontlicense.org
//
// -----------------------------------------------------------------------------
// SIL OPEN FONT LICENSE Version 1.1 - 26 February 2007
// -----------------------------------------------------------------------------
//
// PREAMBLE
// The goals of the Open Font License (OFL) are to stimulate worldwide
// development of collaborative font projects, to support the font creation
// efforts of academic and linguistic communities, and to provide a free and
// open framework in which fonts may be shared and improved in partnership
// with others.
//
// The OFL allows the licensed fonts to be used, studied, modified and
// redistributed freely as long as they are not sold by themselves. The
// fonts, including any derivative works, can be bundled, embedded,
// redistributed and/or sold with any software provided that any reserved
// names are not used by derivative works. The fonts and derivatives,
// however, cannot be released under any other type of license. The
// requirement for fonts to remain under this license does not apply
// to any document created using the fonts or their derivatives.
//
// DEFINITIONS
// "Font Software" refers to the set of files released by the Copyright
// Holder(s) under this license and clearly marked as such. This may
// include source files, build scripts and documentation.
//
// "Reserved Font Name" refers to any names specified as such after the
// copyright statement(s).
//
// "Original Version" refers to the collection of Font Software components as
// distributed by the Copyright Holder(s).
//
// "Modified Version" refers to any derivative made by adding to, deleting,
// or substituting -- in part or in whole -- any of the components of the
// Original Version, by changing formats or by porting the Font Software to a
// new environment.
//
// "Author" refers to any designer, engineer, programmer, technical
// writer or other person who contributed to the Font Software.
//
// PERMISSION & CONDITIONS
// Permission is hereby granted, free of charge, to any person obtaining
// a copy of the Font Software, to use, study, copy, merge, embed, modify,
// redistribute, and sell modified and unmodified copies of the Font
// Software, subject to the following conditions:
//
// 1) Neither the Font Software nor any of its individual components,
// in Original or Modified Versions, may be sold by itself.
//
// 2) Original or Modified Versions of the Font Software may be bundled,
// redistributed and/or sold with any software, provided that each copy
// contains the above copyright notice and this license. These can be
// included either as stand-alone text files, human-readable headers or
// in the appropriate machine-readable metadata fields within text or
// binary files as long as those fields can be easily viewed by the user.
//
// 3) No Modified Version of the Font Software may use the Reserved Font
// Name(s) unless explicit written permission is granted by the corresponding
// Copyright Holder. This restriction only applies to the primary font name as
// presented to the users.
//
// 4) The name(s) of the Copyright Holder(s) or the Author(s) of the Font
// Software shall not be used to promote, endorse or advertise any
// Modified Version, except to acknowledge the contribution(s) of the
// Copyright Holder(s) and the Author(s) or with their explicit written
// permission.
//
// 5) The Font Software, modified or unmodified, in part or in whole,
// must be distributed entirely under this license, and must not be
// distributed under any other license. The requirement for fonts to
// remain under this license does not apply to any document created
// using the Font Software.
//
// TERMINATION
// This license becomes null and void if any of the above conditions are
// not met.
//
// DISCLAIMER
// THE FONT SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
// EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO ANY WARRANTIES OF
// MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT
// OF COPYRIGHT, PATENT, TRADEMARK, OR OTHER RIGHT. IN NO EVENT SHALL THE
// COPYRIGHT HOLDER BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY,
// INCLUDING ANY GENERAL, SPECIAL, INDIRECT, INCIDENTAL, OR CONSEQUENTIAL
// DAMAGES, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING
// FROM, OUT OF THE USE OR INABILITY TO USE THE FONT SOFTWARE OR FROM
// OTHER DEALINGS IN THE FONT SOFTWARE.
package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

// ref pins google/fonts to one commit so a re-run is reproducible.
const ref = "9710da1eacb3be272583c3224dcb70f9da6eadbb"

const base = "https://raw.githubusercontent.com/google/fonts/" + ref + "/"

// source maps a destination file to its pinned source. The URLs are
// percent-encoded because the upstream filenames carry [ ] and ,.
var source = []struct{ dest, src string }{
	{"EBGaramond.ttf", base + "ofl/ebgaramond/EBGaramond%5Bwght%5D.ttf"},
	{"EBGaramond-Italic.ttf", base + "ofl/ebgaramond/EBGaramond-Italic%5Bwght%5D.ttf"},
	{"OFL-EBGaramond.txt", base + "ofl/ebgaramond/OFL.txt"},
	{"Inter.ttf", base + "ofl/inter/Inter%5Bopsz%2Cwght%5D.ttf"},
	{"Inter-Italic.ttf", base + "ofl/inter/Inter-Italic%5Bopsz%2Cwght%5D.ttf"},
	{"OFL-Inter.txt", base + "ofl/inter/OFL.txt"},
	{"JetBrainsMono.ttf", base + "ofl/jetbrainsmono/JetBrainsMono%5Bwght%5D.ttf"},
	{"OFL-JetBrainsMono.txt", base + "ofl/jetbrainsmono/OFL.txt"},
}

func main() {
	dir := "fonts"
	if len(os.Args) > 1 {
		dir = os.Args[1]
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "gen: %v\n", err)
		os.Exit(1)
	}
	for _, entry := range source {
		dest := filepath.Join(dir, entry.dest)
		if err := fetch(entry.src, dest); err != nil {
			fmt.Fprintf(os.Stderr, "gen: %s: %v\n", entry.dest, err)
			os.Exit(1)
		}
		fmt.Printf("wrote %s\n", dest)
	}
}

func fetch(url, dest string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	return os.WriteFile(dest, data, 0644)
}
