package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
)

// GitHubPushEvent 结构体
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

// GitHubPullRequest Event 结构体
type GitHubPREvent struct {
	Action      string `json:"action"`
	PullRequest struct {
		Title  string `json:"title"`
		URL    string `json:"html_url"`
		State  string `json:"state"`
		Merged bool   `json:"merged"`
		Base   struct {
			Ref string `json:"ref"` // 目标分支
		} `json:"base"`
		Head struct {
			Ref string `json:"ref"` // 来源分支
			Sha string `json:"sha"` // 提交 SHA
		} `json:"head"`
	} `json:"pull_request"`
	Repository struct {
		Name string `json:"name"`
	} `json:"repository"`
}

// 获取飞书 Webhook URL
func getFeishuURL(repoName string) string {
	switch repoName {
	case "4UOnline-Go", "4UOnline-Taro":
		return Config.GetString("4u")
	case "JingHong-Questionnaire", "QA-System":
		return Config.GetString("qa")
	case "WeJH-Go", "WeJH-Taro", "JingHong-Admin-Vue":
		return Config.GetString("wjh")
	default:
		return Config.GetString("feishu")
	}
}

// 发送消息到飞书
func sendToFeishu(name, message string) error {
	msg := map[string]interface{}{
		"msg_type": "text",
		"content": map[string]string{
			"text": message,
		},
	}
	msgBytes, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	resp, err := http.Post(getFeishuURL(name), "application/json", bytes.NewBuffer(msgBytes))
	if err != nil {
		return err
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			log.Println("关闭响应体时出错: ", err)
		}
	}(resp.Body)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	fmt.Println("飞书响应: ", string(body))
	return nil
}

// 处理 GitHub 事件
func handleGitHubEvent(c *gin.Context) {
	eventType := c.Request.Header.Get("X-GitHub-Event")
	var message string
	var repoName string

	switch eventType {
	case "push":
		var pushEvent GitHubPushEvent
		if err := c.ShouldBindJSON(&pushEvent); err != nil {
			c.JSON(http.StatusOK, gin.H{"error": "Invalid push request"})
			return
		}

		// 检查分支是否为 main 或 dev
		if !(pushEvent.Ref == "refs/heads/main" || pushEvent.Ref == "refs/heads/dev") {
			c.JSON(http.StatusOK, gin.H{"status": "ignored"})
			return
		}

		repoName = pushEvent.Repository.Name
		repoURL := pushEvent.Repository.URL
		branchName := strings.TrimPrefix(pushEvent.Ref, "refs/heads/")

		message = fmt.Sprintf("项目: %s (%s) 有新的代码更改 (分支: %s):\n", repoName, repoURL, branchName)
		for _, commit := range pushEvent.Commits {
			message += fmt.Sprintf("- 提交: %s\n  信息: %s\n  链接: %s\n", commit.ID[:7], commit.Message, commit.URL)
		}

	case "pull_request":
		var prEvent GitHubPREvent
		if err := c.ShouldBindJSON(&prEvent); err != nil {
			c.JSON(http.StatusOK, gin.H{"error": "Invalid PR request"})
			return
		}

		// 检查目标分支是否为 main 或 dev
		if prEvent.PullRequest.Base.Ref != "main" && prEvent.PullRequest.Base.Ref != "dev" {
			c.JSON(http.StatusOK, gin.H{"status": "ignored"})
			return
		}

		repoName = prEvent.Repository.Name
		sourceBranch := prEvent.PullRequest.Head.Ref
		targetBranch := prEvent.PullRequest.Base.Ref

		switch prEvent.Action {
		case "opened":
			message = fmt.Sprintf("📢 项目: %s 收到新的 Pull Request:\n- 标题: %s\n- 来源分支: %s\n- 目标分支: %s\n- 链接: %s\n",
				repoName, prEvent.PullRequest.Title, sourceBranch, targetBranch, prEvent.PullRequest.URL)
		case "synchronize":
			message = fmt.Sprintf("🔄 项目: %s 的 Pull Request 已更新:\n- 标题: %s\n- 来源分支: %s\n- 目标分支: %s\n- 链接: %s\n",
				repoName, prEvent.PullRequest.Title, sourceBranch, targetBranch, prEvent.PullRequest.URL)
		case "closed":
			if prEvent.PullRequest.Merged {
				message = fmt.Sprintf("✅ 项目: %s 的 Pull Request 已合并:\n- 标题: %s\n- 来源分支: %s\n- 目标分支: %s\n- 链接: %s\n",
					repoName, prEvent.PullRequest.Title, sourceBranch, targetBranch, prEvent.PullRequest.URL)
			} else {
				message = fmt.Sprintf("❌ 项目: %s 的 Pull Request 已关闭:\n- 标题: %s\n- 来源分支: %s\n- 目标分支: %s\n- 链接: %s\n",
					repoName, prEvent.PullRequest.Title, sourceBranch, targetBranch, prEvent.PullRequest.URL)
			}
		default:
			c.JSON(http.StatusOK, gin.H{"status": "ignored"})
			return
		}

	default:
		c.JSON(http.StatusOK, gin.H{"error": "Unsupported event type"})
		return
	}

	if err := sendToFeishu(repoName, message); err != nil {
		c.JSON(http.StatusOK, gin.H{"error": "Failed to send message"})
	} else {
		c.JSON(http.StatusOK, gin.H{"status": "success"})
	}
}

var Config *viper.Viper

func main() {
	Config = viper.New()
	Config.SetConfigName("config")
	Config.SetConfigType("yaml")
	Config.AddConfigPath(".")
	Config.WatchConfig()
	if err := Config.ReadInConfig(); err != nil {
		log.Printf("读取配置文件出错: %v\n", err)
		return
	}

	r := gin.Default()
	r.POST("/webhook/github", handleGitHubEvent)
	err := r.Run(":8008")
	if err != nil {
		log.Printf("启动服务器出错: %v\n", err)
		return
	}
}
