package main

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"
)

type TimeInfo struct {
	Created interface{} `json:"created"`
	Updated interface{} `json:"updated"`
}

type Summary struct {
	Additions int `json:"additions"`
	Deletions int `json:"deletions"`
	Files     int `json:"files"`
}

type Sessions []Session

type Session struct {
	ID        string   `json:"id"`
	Version   string   `json:"version"`
	ProjectID string   `json:"projectID"`
	Directory string   `json:"directory"`
	Title     string   `json:"title"`
	Time      TimeInfo `json:"time"`
	Summary   Summary  `json:"summary"`
}

func getTime(created interface{}) time.Time {
	switch v := created.(type) {
	case float64:
		return time.UnixMilli(int64(v))
	case string:
		if i, err := strconv.ParseInt(v, 10, 64); err == nil {
			return time.UnixMilli(i)
		}
	}
	return time.Time{}
}

func NewSession(filePath string) (Session, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return Session{}, err
	}
	var s Session
	err = json.Unmarshal(data, &s)
	return s, err
}

func (s *Sessions) Scan(dir string) error {
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && filepath.Ext(path) == ".json" {
			session, err := NewSession(path)
			if err != nil {
				log.Printf("Error parsing session file %s: %v", path, err)
				return nil // continue
			}
			*s = append(*s, session)
		}
		return nil
	})
	return err
}

func (s Sessions) Sort() {
	sort.Slice(s, func(i, j int) bool {
		ti := getTime(s[i].Time.Created)
		tj := getTime(s[j].Time.Created)
		return ti.After(tj)
	})
}

// DeleteSession deletes a session by removing its JSON file and associated directories
func DeleteSession(dir string, sessionID string) error {
	// Find and delete the session JSON file
	var sessionPath string
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && filepath.Ext(path) == ".json" {
			session, err := NewSession(path)
			if err != nil {
				return nil // continue
			}
			if session.ID == sessionID {
				sessionPath = path
				return filepath.SkipAll
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	if sessionPath == "" {
		return os.ErrNotExist
	}

	// Delete the session JSON file
	if err := os.Remove(sessionPath); err != nil {
		return err
	}

	// Delete the associated session directory (contains messages, etc.)
	sessionDir := filepath.Join(filepath.Dir(sessionPath), sessionID)
	if _, err := os.Stat(sessionDir); err == nil {
		if err := os.RemoveAll(sessionDir); err != nil {
			log.Printf("Warning: could not delete session directory %s: %v", sessionDir, err)
		}
	}

	return nil
}
