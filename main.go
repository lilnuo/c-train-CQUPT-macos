package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"main/ai"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/chromedp/chromedp"
)

var mu sync.Mutex
var scanner = bufio.NewScanner(os.Stdin)
var score int
var wg sync.WaitGroup

func main() {
	err := ai.InitAI()
	if err != nil {
		log.Fatalf("ai初始化失败")
	}
	fmt.Println("请输入你的学号")
	scanner.Scan()
	username := scanner.Text()
	fmt.Println("请输入你的密码")
	scanner.Scan()
	password := scanner.Text()
	fmt.Println("请一字不差的输入你想刷的章节")
	scanner.Scan()
	choi := scanner.Text()
	fmt.Println("输入你想刷的题目数量")
	scanner.Scan()
	numStr := scanner.Text()
	num, err := strconv.Atoi(numStr)
	wg.Add(num)
	for i := range num {
		go GetScore(choi, username, password, i, &score)
	}
	wg.Wait()
	fmt.Println("最终得分为", score, "分")
}

func GetScore(choi, username, password string, i int, score *int) {
	var scoreStr string
	//创建chrome实例
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", false),
		chromedp.ExecPath("/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"),
	)
	alloctx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancel()
	ctx, cancel := chromedp.NewContext(alloctx)
	defer cancel()
	defer cancel()

	//要刷的章节
	selector := fmt.Sprintf(`//span[contains(text(), "%s")]`, choi)

	err := chromedp.Run(ctx,
		chromedp.Navigate("https://cprg.cqupt.edu.cn/train/#/login"),
		//输入学号
		chromedp.WaitVisible(`input[name="username"]`, chromedp.ByQuery),
		chromedp.SendKeys(`input[name="username"]`, username, chromedp.ByQuery),
		//输入密码
		chromedp.WaitVisible(`input[name="password"]`, chromedp.ByQuery),
		chromedp.SendKeys(`input[name="password"]`, password, chromedp.ByQuery),
		//点击登录
		chromedp.WaitVisible(`button[color="primary"]:not([disabled])`, chromedp.ByQuery),
		chromedp.Click(`button[color="primary"]`, chromedp.ByQuery),
		//进入开始练习页面
		chromedp.WaitVisible(`button[routerlink="choose-mode"]`, chromedp.ByQuery),
		chromedp.Click(`button[routerlink="choose-mode"]`, chromedp.ByQuery),
		//点击要刷的习题
		chromedp.WaitVisible(selector, chromedp.BySearch),
		chromedp.Click(selector, chromedp.BySearch),
		//点击开始
		chromedp.WaitVisible(`div#group button`, chromedp.ByQuery),
		chromedp.Click(`div#group button`, chromedp.ByQuery),
	)
	if err != nil {
		log.Printf("登录失败,error: %v(%v)", err, i)
	}
	fmt.Printf("登陆成功(%v)\n", i)
	//验证是否刷完
	checkCtx, checkCancel := context.WithTimeout(ctx, 1*time.Second)
	defer checkCancel()
	fmt.Printf("正在检查是否已刷完...(%v)\n", i)
	err = chromedp.Run(checkCtx,
		chromedp.WaitVisible(`//div[contains(text(), "你已经完成了")]`, chromedp.BySearch),
	)
	if err == nil {
		log.Printf("你已经完成该标签下的所有题目(%v)\n", i)
		log.Printf("程序退出(%v)\n", i)
		time.Sleep(20 * time.Second)
		wg.Done()
		return
	}
	if err == context.DeadlineExceeded {
		var question string
		//获取题目
		err := chromedp.Run(ctx,
			chromedp.Text(`.q-content`, &question, chromedp.ByQuery),
		)
		if err != nil {
			log.Printf("获取题目失败(%v)\n", i)
			wg.Done()
			return
		}
		fmt.Printf("解题中...(%v)\n", i)
		answer, err := ai.Answer(question)
		if err != nil {
			log.Printf("答案生成失败:err: %v(%v)\n", err, i)
			wg.Done()
			return
		}
		codeBytes, _ := json.Marshal(answer)
		fillCodeJS := fmt.Sprintf(`monaco.editor.getEditors()[0].setValue(%s)`, string(codeBytes))

		err = chromedp.Run(ctx,
			chromedp.WaitVisible(`.monaco-editor`, chromedp.ByQuery),
			chromedp.Evaluate(fillCodeJS, nil),
			chromedp.WaitVisible(`//button[contains(., "提交")]`, chromedp.BySearch),
			chromedp.Click(`//button[contains(., "提交")]`, chromedp.BySearch),
			chromedp.WaitVisible(`//button[contains(., "确定")]`, chromedp.BySearch),
			chromedp.Click(`//button[contains(., "确定")]`, chromedp.BySearch),
			chromedp.WaitVisible(`tr[role="row"] td.mat-column-score`),
			chromedp.Text(`tr[role="row"] td.mat-column-score`, &scoreStr),
		)
		if err != nil {
			log.Printf("填充代码或提交失败: %v(%v)\n", err, i)
			wg.Done()
			return
		}

		fmt.Printf("已解答,得分为%v分(%v)\n", scoreStr, i)
		scoreInt, _ := strconv.Atoi(scoreStr)
		mu.Lock()
		*score += scoreInt
		mu.Unlock()

	} else {
		//出错
		log.Printf("检测过程发生未知错误: %v(%v)\n", err, i)
		wg.Done()
		return
	}
	wg.Done()
}
