package home

import (
	"archive/zip"
	"bytes"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"testing"
)

// Our own text, compressed as Supercell does (5 props, 4-byte size).
const lzmaVector = "5d000001009107000000271849a667ada01ed3bbf9ffad584e0ac09fcafb4358696a0edcaae961dfc1c0a9b2eb88b7f44240c73cb0b1c38a435417a801b9dfc67a718f6a01980cd4be3700c42231d9deea2a29033b6d6abb938f47bdcb7d5cbf07bd72be548309a6bb8d52875c2a6c177de6abc579a6ce7da373ec260ea1e3762075479a5b18d81c96d8f69a7e08dab517e59223b44f96324adbf03a43d4dfd411880e1134a37f397ec84c5881cc53b71f78df2d08ebf30c2ae675961b2c882fcc5edcb113e9853670c47e027a81b7c90380e07e4219e6c5ab5e2a41a3e23cb07740a29095bdf38a3f0555db233a3827103ae014edeab1feada8970b7aa2ddb4d334cba00f662d15cae529e188ba6c18bd4f9195b3ae67345a3170ccbf1226838174e0c430516ef2a86530f999747325231c57ffca6e9be0"

func TestSCDecompress(t *testing.T) {
	b, _ := hex.DecodeString(lzmaVector)
	out, err := scDecompress(b)
	if err != nil {
		t.Fatal(err)
	}
	var want bytes.Buffer
	want.WriteString("Name,Cost\nstring,int\n")
	for i := 0; i < 200; i++ {
		fmt.Fprintf(&want, "Unit%d,%d\n", i, i%5)
	}
	want.WriteString("the end, the end, the end\n")
	if !bytes.Equal(out, want.Bytes()) {
		t.Fatalf("got %d bytes, want %d", len(out), want.Len())
	}
	if plain, _ := scDecompress([]byte("Name,Cost\n")); string(plain) != "Name,Cost\n" {
		t.Fatal("plain text changed")
	}
}

// With CMSERVER_TEST_APK set, every csv_logic table of the APK decodes.
func TestSCDecompressAPK(t *testing.T) {
	apk := os.Getenv("CMSERVER_TEST_APK")
	if apk == "" {
		t.Skip("CMSERVER_TEST_APK not set")
	}
	z, err := zip.OpenReader(apk)
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	n := 0
	for _, f := range z.File {
		if len(f.Name) < 4 || f.Name[len(f.Name)-4:] != ".csv" {
			continue
		}
		r, _ := f.Open()
		b, _ := io.ReadAll(r)
		r.Close()
		out, err := scDecompress(b)
		if err != nil {
			t.Fatalf("%s: %v", f.Name, err)
		}
		if !bytes.HasPrefix(out, []byte("Name")) && !bytes.HasPrefix(out, []byte("\"Name")) && !bytes.HasPrefix(out, []byte("TID")) && !bytes.HasPrefix(out, []byte("\"TID")) {
			t.Logf("%s: starts %q", f.Name, out[:min(len(out), 20)])
		}
		n++
	}
	t.Logf("%d csv files", n)
}
