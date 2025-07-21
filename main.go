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

func ScenarioHeavyRead(app *App) {
	const (
		totalOps    = 1000000 
		readRatio   = 0.9
		concurrency = 500 
	)

	var wg sync.WaitGroup
	wg.Add(concurrency)

	for i := 0; i < concurrency; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < totalOps/concurrency; j++ {
				if float64(j)/float64(totalOps/concurrency) < readRatio {
					app.UserS.Get(id)
				} else {
					app.UserS.Store(id, User{Name: fmt.Sprintf("User-%d", j)})
				}
			}
		}(i)
	}
	wg.Wait()
}

func ScenarioHeavyWrite(app *App) {
	const (
		totalOps    = 1000000 
		writeRatio  = 0.9
		concurrency = 500
	)

	var wg sync.WaitGroup
	wg.Add(concurrency)

	for i := 0; i < concurrency; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < totalOps/concurrency; j++ {
				if float64(j)/float64(totalOps/concurrency) < writeRatio {
					app.UserS.Store(id, User{Name: fmt.Sprintf("User-%d", j)})
				} else {
					app.UserS.Get(id)
				}
			}
		}(i)
	}
	wg.Wait()
}

func ScenarioMixedReadWrite(app *App) {
	const (
		totalOps    = 10000000
		readRatio   = 0.5
		concurrency = 5000
	)

	var wg sync.WaitGroup
	wg.Add(concurrency)

	for i := 0; i < concurrency; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < totalOps/concurrency; j++ {
				if float64(j)/float64(totalOps/concurrency) < readRatio {
					app.UserS.Get(id)
				} else {
					app.UserS.Store(id, User{Name: fmt.Sprintf("User-%d", j)})
				}
			}
		}(i)
	}
	wg.Wait()
}

func runScenario(name string, fn func()) time.Duration {
	const runs = 10
	var total time.Duration
	for i := 0; i < runs; i++ {
		start := time.Now()
		fn()
		elapsed := time.Since(start)
		total += elapsed
	}
	avg := total / runs
	fmt.Printf("%s average time over %d runs: %v\n", name, runs, avg)
	return avg
}

func ScenarioSyncMapHeavyRead() {
	app := CreateApp(true)
	ScenarioHeavyRead(app)
}

func ScenarioSyncMapHeavyWrite() {
	app := CreateApp(true)
	ScenarioHeavyWrite(app)
}

func ScenarioRWMutexHeavyRead() {
	app := CreateApp(false)
	ScenarioHeavyRead(app)
}

func ScenarioRWMutexHeavyWrite() {
	app := CreateApp(false)
	ScenarioHeavyWrite(app)
}

func ScenarioSyncMapMixed() {
	app := CreateApp(true)
	ScenarioMixedReadWrite(app)
}

func ScenarioRWMutexMixed() {
	app := CreateApp(false)
	ScenarioMixedReadWrite(app)
}

func main() {
	fmt.Println("Starting benchmarks...")

	runScenario("sync.Map heavy read", ScenarioSyncMapHeavyRead)
	runScenario("sync.Map heavy write", ScenarioSyncMapHeavyWrite)
	runScenario("RWMutex heavy read", ScenarioRWMutexHeavyRead)
	runScenario("RWMutex heavy write", ScenarioRWMutexHeavyWrite)
	runScenario("sync.Map mixed read/write", ScenarioSyncMapMixed)
	runScenario("RWMutex mixed read/write", ScenarioRWMutexMixed)
}
