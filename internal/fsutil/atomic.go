// Package fsutil agrupa utilidades de escritura segura de archivos.
package fsutil

import (
	"io"
	"os"
	"path/filepath"
)

// WriteFileAtomic escribe data en path de forma atómica: crea un temporal en el
// mismo directorio y lo renombra al final. Así un fallo a mitad de escritura no
// deja el archivo destino corrupto.
func WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, perm); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

// CopyFile copia src sobre dst usando escritura atómica.
func CopyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()

	data, err := io.ReadAll(in)
	if err != nil {
		return err
	}
	return WriteFileAtomic(dst, data, 0o600)
}
