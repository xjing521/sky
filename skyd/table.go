package skyd

import (
	"errors"
	"fmt"
	"hash/fnv"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
)

// A Table is a collection of tablets.
type Table struct {
	Path    string
	Name    string
	Tablets []*Tablet
}

// NewTable returns a new Table that is stored at a given path.
func NewTable(path string) *Table {
	path, err := filepath.Abs(path)
	if err != nil {
		return nil
	}

	return &Table{
		Path: path,
		Name: filepath.Base(path),
	}
}

// Creates a table directory structure.
func (t *Table) Create() error {
	if t.Exists() {
		return fmt.Errorf("Table already exist: %v", t.Path)
	}

	// Create root directory.
	err := os.MkdirAll(t.Path, 0700)
	if err != nil {
		return err
	}

	// Create a subdirectory for each tablet.
	for i := 0; i < runtime.NumCPU(); i++ {
		err = os.Mkdir(fmt.Sprintf("%v/%v", t.Path, i), 0700)
		if err != nil {
			os.RemoveAll(t.Path)
			return nil
		}
	}

	return nil
}

// Opens the table.
func (t *Table) Open() error {
	if !t.Exists() {
		return errors.New("Table does not exist")
	}

	// Create tablets from child directories with numeric names.
	infos, err := ioutil.ReadDir(t.Path)
	if err != nil {
		return err
	}
	for _, info := range infos {
		match, _ := regexp.MatchString("^\\d$", info.Name())
		if info.IsDir() && match {
			tablet := NewTablet(fmt.Sprintf("%s/%s", t.Path, info.Name()))
			t.Tablets = append(t.Tablets, tablet)
			tablet.Open()
		}
	}

	return nil
}

// Closes the table and all the tablets.
func (t *Table) Close() {
	for _, tablet := range t.Tablets {
		tablet.Close()
	}
	t.Tablets = nil
}

// Checks if the table is currently open.
func (t *Table) IsOpen() bool {
	return t.Tablets != nil
}

// Checks if the table exists on disk.
func (t *Table) Exists() bool {
	if _, err := os.Stat(t.Path); os.IsNotExist(err) {
		return false
	}
	return true
}

// Adds an event for a given object to the table.
func (t *Table) AddEvent(objectId interface{}, event *Event) error {
	if !t.IsOpen() {
		return errors.New("Table is not open")
	}

	// Determine tablet number that event should go to.
	tabletIndex, err := t.GetObjectTabletIndex(objectId)
	if err != nil {
		return err
	}

	// Add event to the appropriate tablet.
	tablet := t.Tablets[tabletIndex]
	err = tablet.AddEvent(objectId, event)
	if err != nil {
		return err
	}

	return nil
}

// Retrieves a list of events for a given object.
func (t *Table) GetEvents(objectId interface{}) ([]*Event, error) {
	if !t.IsOpen() {
		return nil, errors.New("Table is not open")
	}

	// Determine tablet number that event should go to.
	tabletIndex, err := t.GetObjectTabletIndex(objectId)
	if err != nil {
		return nil, err
	}

	// Add event to the appropriate tablet.
	tablet := t.Tablets[tabletIndex]
	events, err := tablet.GetEvents(objectId)
	if err != nil {
		return nil, err
	}

	return events, nil
}

// Calculates a tablet index based on the object identifier even hash.
func (t *Table) GetObjectTabletIndex(objectId interface{}) (uint32, error) {
	if !t.IsOpen() {
		return 0, errors.New("Table is not open")
	}

	// Encode object identifier.
	encodedObjectId, err := EncodeObjectId(objectId)
	if err != nil {
		return 0, err
	}

	// Calculate the even bits of the FNV1a hash.
	h := fnv.New64a()
	h.Reset()
	h.Write(encodedObjectId)
	hashcode := h.Sum64()
	index := CondenseUint64Even(hashcode) % uint32(len(t.Tablets))

	return index, nil
}
