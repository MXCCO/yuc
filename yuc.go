package main

import (
    "flag"
    "fmt"
    "log"
    "net/http"
    "net/url"
    "strings"
    "time"

    "github.com/PuerkitoBio/goquery"
    "github.com/valyala/fasthttp"
)

// fetchPageContent 发送 HTTP 请求并获取页面内容
func fetchPageContent(pageURL string) (string, error) {
    req := fasthttp.AcquireRequest()
    defer fasthttp.ReleaseRequest(req)
    req.SetRequestURI(pageURL)

    resp := fasthttp.AcquireResponse()
    defer fasthttp.ReleaseResponse(resp)

    client := &fasthttp.Client{}
    if err := client.Do(req, resp); err != nil {
        return "", err
    }

    body := resp.Body()
    return string(body), nil
}

// cleanText 清理文本内容，去除多余的空白字符
func cleanText(text string) string {
    // 去除所有多余的空白字符，包括空格和空行
    return strings.Join(strings.Fields(text), " ")
}

// parsePostContent 解析帖子内容并获取第一个 id="myshares" 标签内的标题和第一个 class="message" 标签内的文本内容
func parsePostContent(postURL string) (string, string) {
    htmlContent, err := fetchPageContent(postURL)
    if err != nil {
        log.Printf("获取帖子内容失败: %v", err)
        return "", ""
    }

    doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlContent))
    if err != nil {
        log.Printf("解析帖子 HTML 失败: %v", err)
        return "", ""
    }

    // 提取第一个 id="myshares" 标签内的标题
    title := doc.Find("#myshares a").First().Text()

    // 提取第一个 class="message" 标签内的文本内容
    message := doc.Find(".message").First().Text()
    cleanedMessage := cleanText(message)

    if cleanedMessage == "" {
        cleanedMessage = "未找到内容"
    }

    return strings.TrimSpace(title), cleanedMessage
}

// parseForumPage 解析论坛页面内容并获取第一个 .th_item 元素中的链接
func parseForumPage(htmlContent string, baseURL string) (string, string) {
    doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlContent))
    if err != nil {
        log.Fatalf("解析 HTML 失败: %v", err)
    }

    // 提取第一个 .th_item 元素中的链接
    firstPost := doc.Find("a.th_item").First()
    link, exists := firstPost.Attr("href")
    if exists {
        // 确保链接是完整的 URL
        postURL := link
        if !strings.HasPrefix(link, "http") {
            base, err := url.Parse(baseURL)
            if err != nil {
                log.Fatalf("解析 baseURL 失败: %v", err)
            }
            relative, err := url.Parse(link)
            if err != nil {
                log.Fatalf("解析相对链接失败: %v", err)
            }
            postURL = base.ResolveReference(relative).String()
        }

        return postURL, firstPost.Text()
    }
    return "", ""
}

// sendToTelegram 发送消息到Telegram频道
func sendToTelegram(botToken, chatID, message string) error {
    apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", botToken)
    data := url.Values{}
    data.Set("chat_id", chatID)
    data.Set("text", message)

    resp, err := http.PostForm(apiURL, data)
    if err != nil {
        return err
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        return fmt.Errorf("failed to send message to Telegram, status code: %d", resp.StatusCode)
    }

    return nil
}

// monitorForum 持续监控论坛页面
func monitorForum(botToken, chatID string, interval time.Duration) {
    baseURL := "https://fishc.com.cn/forum.php?mod=guide&view=newthread&mobile=2" // 固定的鱼C论坛 URL
    var lastPostURL string

    for {
        // 获取页面内容
        htmlContent, err := fetchPageContent(baseURL)
        if err != nil {
            log.Printf("获取页面内容失败: %v", err)
            time.Sleep(interval)
            continue
        }

        // 解析页面内容并获取第一个 .th_item 元素中的链接
        postURL, _ := parseForumPage(htmlContent, baseURL)
        if postURL != "" && postURL != lastPostURL {
            lastPostURL = postURL

            // 获取帖子内容
            title, message := parsePostContent(postURL)
            telegramMessage := fmt.Sprintf("标题: %s\n链接: %s\n帖子内容: %s", title, postURL, message)
            err := sendToTelegram(botToken, chatID, telegramMessage)
            if err != nil {
                log.Printf("发送消息到Telegram失败: %v", err)
            } else {
                log.Printf("消息已发送到Telegram: %s", telegramMessage)
            }
        }

        time.Sleep(interval)
    }
}

func main() {
    // 定义命令行参数
    botToken := flag.String("token", "", "Telegram Bot API Token")
    chatID := flag.String("chatid", "", "Telegram Chat ID")

    // 解析命令行参数
    flag.Parse()

    // 检查必需的参数是否已提供
    if *botToken == "" || *chatID == "" {
        log.Fatalf("必须提供Telegram Bot API Token和Chat ID")
    }

    // 设置监控间隔时间
    interval := 30 * time.Second

    // 开始监控论坛页面
    monitorForum(*botToken, *chatID, interval)
}