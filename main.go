package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
	"io"
	"net/http"
)

// GitHub Push Event 结构体
type GitHubPushEvent struct {
	Ref     string `json:"ref"`
	Commits []struct {
		ID      string `json:"id"`
		Message string `json:"message"`
		URL     string `json:"url"`
	} `json:"commits"`
	Repository struct {
		Name string `json:"name"`
		URL  string `json:"url"`
	} `json:"repository"`
}

// 发送消息到飞书
func sendToFeishu(message string) error {
	msg := map[string]interface{}{
		"msg_type": "text", // 消息类型为 text
		"content": map[string]string{
			"text": message, // 消息内容为传入的 message
		},
	}

	msgBytes, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	resp, err := http.Post(Url, "application/json", bytes.NewBuffer(msgBytes))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	fmt.Println("飞书响应: ", string(body))
	return nil
}

// 处理 GitHub Push 事件
func handleGitHubPush(c *gin.Context) {
	var pushEvent GitHubPushEvent
	if err := c.ShouldBindJSON(&pushEvent); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	fmt.Println(pushEvent)
	message := fmt.Sprintf("项目: %s 有新的代码更改:\n", pushEvent.Repository.Name)
	for _, commit := range pushEvent.Commits {
		message += fmt.Sprintf("- %s: %s (%s)\n", commit.ID[:7], commit.Message, commit.URL)
	}

	if err := sendToFeishu(message); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to send message"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "success"})
}

var Config *viper.Viper
var Url string

func main() {
	Config = viper.New()
	Config.SetConfigName("config")
	Config.SetConfigType("yaml")
	Config.AddConfigPath(".")
	Config.WatchConfig()
	if err := Config.ReadInConfig(); err != nil {
		fmt.Printf("读取配置文件出错: %v\n", err)
		return
	}
	Url = Config.GetString("feishu")
	r := gin.Default()
	r.POST("/webhook/github", handleGitHubPush)
	r.Run(":8008")
}
