package minecraftspark

import (
	"archive/zip"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strings"
)

// RequiredJavaMajor returns the highest base class-file Java requirement in one Jar.
func RequiredJavaMajor(jarPath string) (int, error) {
	archive, err := zip.OpenReader(jarPath)
	if err != nil {
		return 0, err
	}
	defer func() { _ = archive.Close() }()

	maximum := 0
	for _, file := range archive.File {
		name := strings.ReplaceAll(file.Name, "\\", "/")
		if !strings.HasSuffix(strings.ToLower(name), ".class") || strings.HasPrefix(strings.ToUpper(name), "META-INF/VERSIONS/") {
			continue
		}
		reader, openErr := file.Open()
		if openErr != nil {
			return 0, openErr
		}
		header := make([]byte, 8)
		_, readErr := io.ReadFull(reader, header)
		_ = reader.Close()
		if readErr != nil {
			return 0, readErr
		}
		if binary.BigEndian.Uint32(header[:4]) != 0xCAFEBABE {
			return 0, fmt.Errorf("invalid class file magic: %s", name)
		}
		classMajor := int(binary.BigEndian.Uint16(header[6:8]))
		if classMajor < 45 {
			return 0, fmt.Errorf("unsupported class file major %d: %s", classMajor, name)
		}
		javaMajor := classMajor - 44
		if javaMajor > maximum {
			maximum = javaMajor
		}
	}
	if maximum == 0 {
		return 0, errors.New("jar does not contain base class files")
	}
	return maximum, nil
}
