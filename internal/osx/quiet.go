// Package osx holds thin os helpers for best-effort cleanup paths.
package osx

import "os"

// Remove is best-effort os.Remove. Callers already returning a primary error
// use this so cleanup failure does not hide or replace that error.
func Remove(path string) {
	if err := os.Remove(path); err != nil {
		return
	}
}

// RemoveAll is best-effort os.RemoveAll (same rationale as Remove).
func RemoveAll(path string) {
	if err := os.RemoveAll(path); err != nil {
		return
	}
}
