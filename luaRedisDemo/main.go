package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// 商品表
type Goods struct {
	Id      uint   `gorm:"column:id;type:int(11) unsigned;primary_key;AUTO_INCREMENT" json:"id"`
	Name    string `gorm:"column:name;type:varchar(50);NOT NULL" json:"name"`   // 名称
	Count   int    `gorm:"column:count;type:int(11);NOT NULL" json:"count"`     // 库存
	Sale    int    `gorm:"column:sale;type:int(11);NOT NULL" json:"sale"`       // 已售
	Version int    `gorm:"column:version;type:int(11);NOT NULL" json:"version"` // 乐观锁，版本号
}

// 订单表
type GoodsOrder struct {
	Id         uint      `gorm:"column:id;type:int(11) unsigned;primary_key;AUTO_INCREMENT" json:"id"`
	Gid        int       `gorm:"column:gid;type:int(11);NOT NULL" json:"gid"`                                             // 库存ID
	Name       string    `gorm:"column:name;type:varchar(30);NOT NULL" json:"name"`                                       // 商品名称
	CreateTime time.Time `gorm:"column:create_time;type:timestamp;default:CURRENT_TIMESTAMP;NOT NULL" json:"create_time"` // 创建时间
}

// 实际表名
func (m *GoodsOrder) TableName() string {
	return "goods_order"
}

const orderSet = "orderSet"     //用户id的集合
const goodsTotal = "goodsTotal" //商品库存的key
const orderList = "orderList"   //订单队列
func createScript() *redis.Script {
	str, err := os.ReadFile("./luaRedisDemo/lua-case/luaScript.lua")
	if err != nil {
		fmt.Println("Script read error", err)
		log.Println(err)
	}
	scriptStr := fmt.Sprintf("%s", str)
	script := redis.NewScript(scriptStr)
	return script
}

func evalScript(client *redis.Client, userId string, wg *sync.WaitGroup) {
	defer wg.Done()
	ctx := context.Background()
	script := createScript()
	//fmt.Printf("%+v",script)
	//return
	sha, err := script.Load(ctx, client).Result()
	if err != nil {
		log.Fatalln(err)
	}
	ret := client.EvalSha(ctx, sha, []string{
		userId,
		orderSet,
	}, []string{
		goodsTotal,
		orderList,
	})
	if result, err := ret.Result(); err != nil {
		log.Fatalf("Execute Redis fail: %v", err.Error())
	} else {
		total := result.(int64)
		if total == 0 {
			fmt.Printf("userid: %s, 什么都没抢到 \n", userId)
		} else {
			fmt.Printf("userid: %s 抢到了, 库存: %d \n", userId, total)

		}
	}
}

func main() {
	http.HandleFunc("/", addOrder)
	log.Fatal(http.ListenAndServe(":8082", nil))
}

func getDb() *gorm.DB {
	connArgs := fmt.Sprintf("%s:%s@(%s:%d)/%s?charset=utf8&parseTime=True&loc=Local", "root", "1234", "localhost", 3306, "go-project")
	db, err := gorm.Open(mysql.Open(connArgs), &gorm.Config{
		// 开启Log模式
		Logger: logger.Default.LogMode(logger.Info), // 或者使用 logger.Silent 关闭日志
	})
	if err != nil {
		panic(err)
	}
	sqlDB, _ := db.DB()
	//开启连接池
	sqlDB.SetMaxIdleConns(100)   //最大空闲连接
	sqlDB.SetMaxOpenConns(100)   //最大连接数
	sqlDB.SetConnMaxLifetime(30) //最大生存时间(s)
	return db
}

func getRedis() *redis.Client {
	client := redis.NewClient(&redis.Options{
		Addr: "127.0.0.1:6379",
	})
	return client
}

func addOrder(w http.ResponseWriter, r *http.Request) {
	var wg sync.WaitGroup
	wg.Add(1)
	client := getRedis()

	defer r.Body.Close()
	defer client.Close()

	r.ParseForm()
	uid := r.FormValue("uid")

	go evalScript(client, uid, &wg)
	wg.Wait()
}
