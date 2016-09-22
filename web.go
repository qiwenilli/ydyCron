package main

import (
	"bufio"
	"flag"
	"fmt"
	"html/template"
	"io"
	"io/ioutil"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"time"
	// "os"
	"os/exec"
	//"errors"
	"crypto/md5"
	"sync"

	"github.com/go-xorm/xorm"
	_ "github.com/mattn/go-sqlite3"

	"github.com/fatih/color"
	"github.com/qiniu/log"
)

type GlobalConfig struct {
	ServerPort string
}

type ScheduleTime struct {
	//分1-59 小时(1-24) 日(1-31) 月(1-12) 星期（0-6）
	I, H, D, M, W string
}

type Task struct {
	Id      int
	Name    string
	Settime string
	Cmd     string
	Desc    string
	Status  int
	Ctime   int
}

type Task_history struct {
	Id         int
	Task_id    int
	Start_time int64
	End_time   int64
	Output     string
}

type Task_process struct {
	Pid          int
	Output       string
	Stime, Etime int64
    ExCmd *interface{}
}

var (
	gcfg   GlobalConfig
	engine *xorm.Engine
	lmap   = new(sync.Mutex)
	//
	gprocess = make(map[int]*Task_process, 1024)
)

func main() {
	var err error
	engine, err = xorm.NewEngine("sqlite3", "./cron.db")
	if err != nil {
		color.Red("db connect error!")
		return
	}
	engine.Sync2(new(Task), new(Task_history))

	//crontab task
	// ticker := time.NewTicker(time.Millisecond * 1000*60)
	ticker := time.NewTicker(time.Millisecond * 1000)
	go func() {
		for t := range ticker.C {
			color.Green(fmt.Sprintln("---", t, time.Now()))
			//
			run_task()
		}
	}()

	//
	flag.StringVar(&gcfg.ServerPort, "port", "8412", "port to listen")
	flag.Parse()

	fmt.Println("https://127.0.0.1:" + gcfg.ServerPort)

	// http.Post("/media", uploadHandle)
	http.HandleFunc("/", defaultHandle)
	http.HandleFunc("/web_runing_task", web_runing_task_handle)
	http.HandleFunc("/web_kill_task", web_kill_task_handle)
	http.HandleFunc("/web_task_history", web_task_history_handle)
	http.HandleFunc("/web_task_history_desc", web_task_history_desc_handle)

	//
	server := &http.Server{
		Addr: ":" + gcfg.ServerPort,
		// Handler:        handler,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   30 * time.Second,
		MaxHeaderBytes: 1 << 10,
	}

	err = server.ListenAndServe()
	// err = server.ListenAndServeTLS("./ex/jd.crt", "./ex/jd.key")
	if err != nil {
		log.Debug("....")
	}
}

func run_task() {
	list, err := get_all_task()

	if err != nil {
		color.Red(fmt.Sprintln("%s", err))
		return
	}

	var schedule ScheduleTime

	for _, _task := range list {

		_schedule_str := strings.Fields(_task.Settime)
		if len(_schedule_str) != 5 || _task.Status == 0{
			color.Red("error schedule: " + fmt.Sprintf("%s", _task.Settime))
			continue
		}

		//
		schedule.I = _schedule_str[0]
		schedule.H = _schedule_str[1]
		schedule.D = _schedule_str[2]
		schedule.M = _schedule_str[3]
		schedule.W = _schedule_str[4]

		t := time.Now()
		h, i, _ := t.Clock()
		_, m, d := t.Date()
		w := t.Weekday()

		if false == check_schedule_time(schedule.I, i) {
			continue
		}

		if false == check_schedule_time(schedule.H, h) {
			continue
		}

		if false == check_schedule_time(schedule.D, d) {
			continue
		}

		if false == check_schedule_time(schedule.M, int(m)) {
			continue
		}

		if false == check_schedule_time(schedule.W, int(w)) {
			continue
		}

		fmt.Println("---1---", _task.Name, _task.Cmd)

		// defer
		go process_cmd(_task)
	}
}

func process_cmd(_task Task) {
	var err error

    //判断相同任务是否运行中; 运行中，不再重复执行
	_, ok := gprocess[_task.Id]
	if ok {
       return 
    }
    //创建任务记录对象
    gprocess[_task.Id] = new(Task_process)

	//
	switch runtime.GOOS {
	case "windows":
		err = execute(_task.Id, "cmd", []string{_task.Cmd})
	case "linux":
		fallthrough
	default:
		err = execute(_task.Id, "/bin/bash", []string{"-c", _task.Cmd})
	}

	if err == nil {
		lmap.Lock()

        //如果进程不在，就不再记录
        _, ok := gprocess[_task.Id]
        if !ok {
            return
        }
        //end...

		//记录执行的历史
		fmt.Println(gprocess[_task.Id])

		_task_history := new(Task_history)

		_, err = engine.Desc("id").Get(_task_history)
		if err == nil {
			_task_history.Id++
			_task_history.Task_id = _task.Id
			_task_history.Output = gprocess[_task.Id].Output
			_task_history.Start_time = gprocess[_task.Id].Stime
			_task_history.End_time = gprocess[_task.Id].Etime

			//
			affected, err := engine.Insert(_task_history)
			if err != nil {
				fmt.Println(affected, err)
			}
		}
        //用完删除
        delete(gprocess, _task.Id)

		lmap.Unlock()
	}
}

func get_all_task() (map[int]Task, error) {

	var list = make(map[int]Task)

	task := new(Task)
	// rows, err := engine.Where("Status = ?", 1).Rows(task)
	rows, err := engine.Rows(task)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for i := 0; rows.Next(); i++ {
		err = rows.Scan(task)
		if err != nil {
			return nil, err
			break
		}

		list[i] = Task{
			Id:      task.Id,
			Name:    fmt.Sprintf(task.Name),
			Settime: fmt.Sprintf(task.Settime),
			Cmd:     fmt.Sprintf(task.Cmd),
			Desc:    fmt.Sprintf(task.Desc),
			Status:  task.Status,
			Ctime:   task.Ctime,
		}
	}

	return list, nil
}

func get_all_task_history(task_id int) ([]Task_history, error) {

	var list []Task_history

    err := engine.Where("task_id = ?", task_id).Desc("id").Limit(50,0).Find(&list)
	if err != nil {
		return nil, err
	}

	return list, nil
}


func check_schedule_time(str string, tn int) bool {

	if str == "*" {
		return true
	} else if strings.Index(str, "/") > -1 {
		a := strings.Split(str, "/")
		ai, _ := strconv.Atoi(a[1])

		if tn%ai == 0 {
			return true
		} else {
			return false
		}

	} else if strings.Index(str, "-") > -1 {
		a := strings.Split(str, "-")
		am, _ := strconv.Atoi(a[0])
		an, _ := strconv.Atoi(a[1])

		for i := am; i <= an; i++ {
			if tn == i {
				return true
			}
		}
		return false

	} else if strings.Index(str, ",") > -1 {
		a := strings.Split(str, ",")

		for _, i := range a {
			ai, _ := strconv.Atoi(i)
			if tn == ai {
				return true
			}
			return false
		}

	} else {
		ai, _ := strconv.Atoi(str)
		if tn == ai {
			return true
		}
		return false
	}

	return false
}

func execute(task_id int, command string, args []string) (err error) {

	// stime := fmt.Sprintf(time.Now().Format("2006-01-02 15:04:05"))
	//
	cmd := exec.Command(command, args...)
	//
	stdout, err := cmd.StdoutPipe()
    defer stdout.Close()
	if err != nil {
        gprocess[task_id].Output = fmt.Sprintln(err)
		return
	}
    //
	err = cmd.Start()
	if err != nil {
        gprocess[task_id].Output = fmt.Sprintln(err)
		return
	} else {
        gprocess[task_id].Pid = cmd.Process.Pid 
        gprocess[task_id].Stime = time.Now().Unix()
	}

	//read cmd execute output
	r := bufio.NewReader(stdout)
	_output, err := ioutil.ReadAll(r)
	if err != nil {
        gprocess[task_id].Output = fmt.Sprintln(err)
		return
	}
	output := string(_output)

	//process run ...
    gprocess[task_id].Output = output 

	//
	err = cmd.Wait()
	if err != nil {
        gprocess[task_id].Output = fmt.Sprintln(err)
	}
	//process end ...
    gprocess[task_id].Etime = time.Now().Unix()

	return nil
}

func md5string(str string) string {

	hash := md5.New()
	b := []byte(str)
	hash.Write(b)

	// return fmt.Sprintf("%x", hash.Sum(nil), md5.Sum(b))
	return fmt.Sprintf("%x", md5.Sum(b))
}

//------------------* web ------------------

func defaultHandle(w http.ResponseWriter, r *http.Request) {
	//http.NotFoundHandler()

	//http.Error(w, "403", 403)
	//http.Redirect(w, r, "http://www.jindanlicai.com", 302)
	//return

	w.Header().Set("Server", "nginx 7.0")
	w.Header().Set("location", "http://www.jindanlicai.com")
	io.WriteString(w, "Welcome!")
}

func web_runing_task_handle(w http.ResponseWriter, r *http.Request) {

	t, _ := template.ParseFiles("./ex/tpl/index.html")

	data := make(map[string]interface{})

	_task_list, _ := get_all_task()

    _html_task_list := make(map[int]interface{},len(_task_list))
    for i, v := range _task_list {
        var filed = make(map[string]interface{})
        //
        filed["Id"] = v.Id
        filed["Name"] = v.Name
        filed["Settime"] = v.Settime
        filed["Cmd"] = v.Cmd
        filed["Ctime"] = v.Ctime
        filed["Desc"] = v.Desc
        filed["Status"] = v.Status

        filed["Pid"] = 0
        _, ok := gprocess[v.Id]
        if ok {
            filed["Pid"]  = gprocess[v.Id].Pid
            filed["Stime"] = time.Unix(gprocess[v.Id].Stime, 0).Format("2006-01-02 15:04:05")
        }

        _html_task_list[i] = filed
    }

	data["Html_task_list"] = _html_task_list 
	
    data["Task_process"] = gprocess 

    t.Execute(w, data)
}

func web_kill_task_handle(w http.ResponseWriter, r *http.Request) {

    task_id := r.FormValue("task_id")

    task_id_int, _ := strconv.Atoi(task_id)

    _, ok := gprocess[task_id_int]
    if ok {
        out, _ := exec.Command("/bin/bash","-c", fmt.Sprintf("kill -9 %s", gprocess[task_id_int].Pid)).Output()

        fmt.Println(out)
        
        delete(gprocess, task_id_int)
    }

    //io.WriteString(w, strconv.Itoa(gprocess[task_id_int].Pid))
    io.WriteString(w, task_id)
}

func web_task_history_handle(w http.ResponseWriter, r *http.Request) {

    task_id := r.FormValue("task_id")
    
    task_id_int, _ := strconv.Atoi(task_id)

	t, _ := template.ParseFiles("./ex/tpl/history_list.html")

	data := make(map[string]interface{})

	_task_list, _ := get_all_task_history(task_id_int)

    _html_task_list := make(map[int]interface{},len(_task_list))
    for i, v := range _task_list {
        var filed = make(map[string]interface{})
        //
        filed["Id"] = v.Id
        filed["Task_id"] = v.Task_id
        filed["Start_time"] = time.Unix(v.Start_time, 0).Format("2006-01-02 15:04:05")
        filed["End_time"] = time.Unix(v.End_time, 0).Format("2006-01-02 15:04:05")
        filed["Ex_time"] = v.End_time-v.Start_time

        _html_task_list[i] = filed
    }
	data["Html_task_id"] = task_id 
	data["Html_task_history_list"] = _html_task_list 
	
    t.Execute(w, data)
}

func web_task_history_desc_handle(w http.ResponseWriter, r *http.Request) {

    id := r.FormValue("id")
    
	t, _ := template.ParseFiles("./ex/tpl/history_desc.html")

	data := make(map[string]interface{})

    var task_history Task_history
    
    _,err := engine.Where("id = ?", id).Get(&task_history)
	if err != nil {
	
    }

	data["Html_task_history_desc"] = task_history
	
    t.Execute(w, data)
}

