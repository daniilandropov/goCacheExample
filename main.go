package main

import (
	"bytes"
	"fmt"
	"log"
	"sync"
	"time"
)

const (
	foundInCache    = "found in cache!"
	notFoundInCache = "not found in cache"
	foundInDB       = "found in db!!"
	notFoundInDB    = "not found in db"
	appIsStarted    = "app is started!"
)

type User struct {
	Name string
}

type UserRepo struct {
	o         sync.Once
	dbMutex   sync.Mutex
	db        map[int]User
	isCacheSM bool
	cacheSM   sync.Map
	rwm       sync.RWMutex
	cacheRWM  map[int]User
	logger    *log.Logger
}

func (u *UserRepo) getFromCache(id int) (User, bool) {
	if u.isCacheSM {
		user, ok := u.cacheSM.Load(id)
		if !ok {
			u.logger.Println(notFoundInCache)
			return User{}, false
		}
		u.logger.Println(foundInCache)
		return user.(User), true
	}

	u.rwm.RLock()
	user, ok := u.cacheRWM[id]
	u.rwm.RUnlock()
	if !ok {
		u.logger.Println(notFoundInCache)
		return User{}, false
	}

	u.logger.Println(foundInCache)

	return user, true
}

func (u *UserRepo) storeInCache(id int, user User) {
	if u.isCacheSM {
		u.cacheSM.Store(id, user)
		return
	}

	u.rwm.Lock()
	u.cacheRWM[id] = user
	u.rwm.Unlock()
}

func (u *UserRepo) Get(id int) (User, bool) {
	if v, ok := u.getFromCache(id); ok {
		return v, true
	}

	u.dbMutex.Lock()
	user, ok := u.db[id]
	u.dbMutex.Unlock()
	if !ok {
		u.logger.Println(notFoundInDB)
		return User{}, false
	}

	u.storeInCache(id, user)

	u.logger.Println(foundInDB)

	return user, true
}

func (u *UserRepo) Store(id int, user User) {
	u.dbMutex.Lock()
	u.db[id] = user
	u.dbMutex.Unlock()
	u.storeInCache(id, user)
}

func (u *UserRepo) Init(isCacheSM bool, logger *log.Logger) {
	u.isCacheSM = isCacheSM
	u.logger = logger
	u.o.Do(u.doInit)
}

func (u *UserRepo) doInit() {
	u.db = make(map[int]User)
	u.cacheRWM = make(map[int]User)
}

type UserService struct {
	repo *UserRepo
}

func (u *UserService) Init(r *UserRepo) {
	u.repo = r
}

func (u *UserService) Get(id int) (User, bool) {
	return u.repo.Get(id)
}

func (u *UserService) Store(id int, user User) {
	u.repo.Store(id, user)
}

type UserServer struct {
	service *UserService
}

func (u *UserServer) Init(service *UserService) {
	u.service = service
}

func (u *UserServer) Get(id int) (User, bool) {
	return u.service.Get(id)
}

func (u *UserServer) Store(id int, user User) {
	u.service.Store(id, user)
}

type App struct {
	o         sync.Once
	buf       bytes.Buffer
	logger    *log.Logger
	UserS     UserServer
	isCacheSM bool
}

func (a *App) Init(isCacheSM bool) {
	a.isCacheSM = isCacheSM
	a.o.Do(a.doInit)
}

func (a *App) doInit() {
	a.logger = log.New(&a.buf, "", log.LstdFlags)
	userRepo := UserRepo{}
	userRepo.Init(a.isCacheSM, a.logger)

	userService := UserService{}
	userService.Init(&userRepo)

	userServer := UserServer{}
	userServer.Init(&userService)
	a.UserS = userServer

	a.logger.Println(appIsStarted)
}

func (a *App) Println() {
	fmt.Println(a.buf.String())
}

func CreateApp(isCacheSM bool) *App {
	app := App{}
	app.Init(isCacheSM)

	return &app
}

type Scale struct {
	name        string
	totalOps    int
	concurrency int
	readRatio   float64
	writeRatio  float64
}

var smallScale = Scale{"small", 10000, 50, 0.9, 0.9}
var mediumScale = Scale{"medium", 100000, 200, 0.9, 0.9}
var largeScale = Scale{"large", 10000000, 5000, 0.5, 0.5}

func RunScenario(app *App, scale Scale) {
	var wg sync.WaitGroup
	wg.Add(scale.concurrency)

	for i := 0; i < scale.concurrency; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < scale.totalOps/scale.concurrency; j++ {
				ratio := float64(j) / float64(scale.totalOps/scale.concurrency)
				if ratio < scale.readRatio {
					app.UserS.Get(id)
				} else {
					app.UserS.Store(id, User{Name: fmt.Sprintf("User-%d", j)})
				}
			}
		}(i)
	}
	wg.Wait()
}

func BenchmarkScenario(name string, isCacheSM bool, scale Scale) {
	var total time.Duration
	for i := 0; i < 10; i++ {
		app := CreateApp(isCacheSM)
		start := time.Now()
		RunScenario(app, scale)
		total += time.Since(start)
	}
	fmt.Printf("%s (%s) average time over 10 runs: %v\n", name, scale.name, total/10)
}

func main() {
	fmt.Println("Starting benchmarks...")

	for _, scale := range []Scale{smallScale, mediumScale, largeScale} {
		fmt.Printf("\n--- %s scale ---\n", scale.name)
		BenchmarkScenario("sync.Map heavy read", true, Scale{scale.name, scale.totalOps, scale.concurrency, 0.9, 0.1})
		BenchmarkScenario("sync.Map heavy write", true, Scale{scale.name, scale.totalOps, scale.concurrency, 0.1, 0.9})
		BenchmarkScenario("RWMutex heavy read", false, Scale{scale.name, scale.totalOps, scale.concurrency, 0.9, 0.1})
		BenchmarkScenario("RWMutex heavy write", false, Scale{scale.name, scale.totalOps, scale.concurrency, 0.1, 0.9})
		BenchmarkScenario("sync.Map mixed read/write", true, Scale{scale.name, scale.totalOps, scale.concurrency, 0.5, 0.5})
		BenchmarkScenario("RWMutex mixed read/write", false, Scale{scale.name, scale.totalOps, scale.concurrency, 0.5, 0.5})
	}
}
