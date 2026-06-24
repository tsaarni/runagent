// Provides persistent storage and lookup of managed processes by ID or name.

package daemon

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
)

type Registry struct {
	mutex     sync.Mutex
	processes map[int]*Process
	nextID    int
	path      string
}

type registryData struct {
	Processes map[int]*Process `json:"processes"`
	NextID    int              `json:"next_id"`
}

func NewRegistry(path string) *Registry {
	return &Registry{
		processes: make(map[int]*Process),
		nextID:    1,
		path:      path,
	}
}

func (r *Registry) Load() error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	data, err := os.ReadFile(r.path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}

	var d registryData
	if err := json.Unmarshal(data, &d); err != nil {
		return err
	}

	r.processes = d.Processes
	if r.processes == nil {
		r.processes = make(map[int]*Process)
	}
	r.nextID = d.NextID
	return nil
}

func (r *Registry) Save() error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	data, err := json.Marshal(registryData{Processes: r.processes, NextID: r.nextID})
	if err != nil {
		return err
	}

	tmp := r.path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		if err := os.MkdirAll(filepath.Dir(r.path), 0o755); err != nil {
			return err
		}
		f, err = os.Create(tmp)
		if err != nil {
			return err
		}
	}

	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return err
	}
	_ = f.Close()

	return os.Rename(tmp, r.path)
}

func (r *Registry) Add(p *Process) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	p.ID = r.nextID
	r.nextID++
	r.processes[p.ID] = p
}

func (r *Registry) Remove(id int) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	delete(r.processes, id)
}

func (r *Registry) Get(id int) *Process {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	return r.processes[id]
}

func (r *Registry) FindByName(name string) *Process {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	for _, p := range r.processes {
		if p.Name == name {
			return p
		}
	}
	return nil
}

func (r *Registry) Resolve(target string) *Process {
	if p := r.FindByName(target); p != nil {
		return p
	}
	id, err := strconv.Atoi(target)
	if err != nil {
		return nil
	}
	return r.Get(id)
}

func (r *Registry) All() []*Process {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	all := make([]*Process, 0, len(r.processes))
	for _, p := range r.processes {
		all = append(all, p)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].ID < all[j].ID })
	return all
}

func (r *Registry) NextID() int {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	return r.nextID
}
