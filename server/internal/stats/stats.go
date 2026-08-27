package stats

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

type dayStat struct {
	Date    string          `json:"date"`
	Views   int64           `json:"views"`
	Uniques map[string]bool `json:"-"`
	UniqN   int             `json:"uniques"`
}

type totals struct {
	LoginsOK   int64 `json:"loginsOk"`
	LoginsFail int64 `json:"loginsFail"`
	MailListed int64 `json:"mailsListed"`
	MailOpened int64 `json:"mailsOpened"`
	Sends      int64 `json:"sends"`
	Replies    int64 `json:"replies"`
	Forwards   int64 `json:"forwards"`
	APITotal   int64 `json:"apiTotal"`
	Views      int64 `json:"views"`
}

type Store struct {
	mu        sync.Mutex
	file      string
	startedAt time.Time
	days      map[string]*dayStat
	endpoints map[string]int64
	tot       totals
}

func New(file string) *Store {
	if file == "" {
		file = "stats.json"
	}
	s := &Store{
		file:      file,
		startedAt: time.Now(),
		days:      map[string]*dayStat{},
		endpoints: map[string]int64{},
	}
	s.load()
	return s
}

func (s *Store) load() {
	raw, err := os.ReadFile(s.file)
	if err != nil {
		return
	}
	var disk struct {
		StartedAt time.Time         `json:"startedAt"`
		Days      []dayStat         `json:"days"`
		Uniques   map[string][]string `json:"uniques"`
		Endpoints map[string]int64  `json:"endpoints"`
		Totals    totals            `json:"totals"`
	}
	if json.Unmarshal(raw, &disk) != nil {
		return
	}
	if !disk.StartedAt.IsZero() {
		s.startedAt = disk.StartedAt
	}
	s.tot = disk.Totals
	for _, day := range disk.Days {
		copyDay := day
		copyDay.Uniques = map[string]bool{}
		for _, id := range disk.Uniques[day.Date] {
			copyDay.Uniques[id] = true
		}
		copyDay.UniqN = len(copyDay.Uniques)
		s.days[day.Date] = &copyDay
	}
	s.endpoints = disk.Endpoints
	if s.endpoints == nil {
		s.endpoints = map[string]int64{}
	}
}

func (s *Store) Save() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.saveLocked()
}

func (s *Store) saveLocked() {
	dates := make([]string, 0, len(s.days))
	for date := range s.days {
		dates = append(dates, date)
	}
	sort.Strings(dates)
	days := make([]dayStat, 0, len(dates))
	uniques := map[string][]string{}
	for _, date := range dates {
		day := s.days[date]
		ids := make([]string, 0, len(day.Uniques))
		for id := range day.Uniques {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		uniques[date] = ids
		days = append(days, dayStat{Date: day.Date, Views: day.Views, UniqN: len(ids)})
	}
	disk := struct {
		StartedAt time.Time           `json:"startedAt"`
		Days      []dayStat           `json:"days"`
		Uniques   map[string][]string `json:"uniques"`
		Endpoints map[string]int64    `json:"endpoints"`
		Totals    totals              `json:"totals"`
	}{StartedAt: s.startedAt, Days: days, Uniques: uniques, Endpoints: s.endpoints, Totals: s.tot}
	encoded, err := json.Marshal(disk)
	if err != nil {
		return
	}
	tmp := s.file + ".tmp"
	if os.MkdirAll(filepath.Dir(s.file), 0o755) != nil {
		return
	}
	if os.WriteFile(tmp, encoded, 0o600) != nil {
		return
	}
	_ = os.Rename(tmp, s.file)
}

func (s *Store) StartAutoSave(interval time.Duration) chan struct{} {
	stop := make(chan struct{})
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				s.Save()
			case <-stop:
				s.Save()
				return
			}
		}
	}()
	return stop
}

func today() string {
	return time.Now().Format("2006-01-02")
}

func (s *Store) day(date string) *dayStat {
	day, ok := s.days[date]
	if !ok {
		day = &dayStat{Date: date, Uniques: map[string]bool{}}
		s.days[date] = day
	}
	return day
}

func (s *Store) trimLocked() {
	if len(s.days) <= 120 {
		return
	}
	dates := make([]string, 0, len(s.days))
	for date := range s.days {
		dates = append(dates, date)
	}
	sort.Strings(dates)
	for _, date := range dates[:len(dates)-120] {
		delete(s.days, date)
	}
}

func (s *Store) HitView(visitorID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := HashID(visitorID)
	day := s.day(today())
	day.Views++
	s.tot.Views++
	if !day.Uniques[id] {
		day.Uniques[id] = true
		day.UniqN = len(day.Uniques)
	}
	s.trimLocked()
}

func (s *Store) HitAPI(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.endpoints[path]++
	s.tot.APITotal++
}

func (s *Store) MarkLogin(ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ok {
		s.tot.LoginsOK++
	} else {
		s.tot.LoginsFail++
	}
}

func (s *Store) AddMailListed(n int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tot.MailListed += n
}

func (s *Store) MarkMailOpened() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tot.MailOpened++
}

func (s *Store) MarkSend(kind string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch kind {
	case "reply":
		s.tot.Replies++
	case "forward":
		s.tot.Forwards++
	default:
		s.tot.Sends++
	}
}

type Endpoint struct {
	Path  string `json:"path"`
	Count int64  `json:"count"`
}

type DayRow struct {
	Date    string `json:"date"`
	Views   int64  `json:"views"`
	Uniques int    `json:"uniques"`
}

type Snapshot struct {
	StartedAt time.Time  `json:"startedAt"`
	Now       time.Time  `json:"now"`
	Totals    totals     `json:"totals"`
	Days      []DayRow   `json:"days"`
	Endpoints []Endpoint `json:"endpoints"`
}

func (s *Store) Snapshot() Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	dates := make([]string, 0, len(s.days))
	for date := range s.days {
		dates = append(dates, date)
	}
	sort.Strings(dates)
	if len(dates) > 30 {
		dates = dates[len(dates)-30:]
	}
	days := make([]DayRow, 0, len(dates))
	for _, date := range dates {
		day := s.days[date]
		days = append(days, DayRow{Date: date, Views: day.Views, Uniques: day.UniqN})
	}
	endpoints := make([]Endpoint, 0, len(s.endpoints))
	for path, count := range s.endpoints {
		endpoints = append(endpoints, Endpoint{Path: path, Count: count})
	}
	sort.Slice(endpoints, func(i, j int) bool { return endpoints[i].Count > endpoints[j].Count })
	if len(endpoints) > 30 {
		endpoints = endpoints[:30]
	}
	return Snapshot{
		StartedAt: s.startedAt,
		Now:       time.Now(),
		Totals:    s.tot,
		Days:      days,
		Endpoints: endpoints,
	}
}

func HashID(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:8])
}

func SafeEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
