package main

import (
	"fmt"
	"io"
	"net/http"
    "net/url"
	"regexp"
	"strings"
	"sync"
	"time"
	"os"
    "log"
    "flag"
    "path/filepath"
    "encoding/json"

	"github.com/antchfx/htmlquery"
)


// 引数
var startURL string
var maxDepth int
var outputDirPath string

// 訪問済みURLを記録するためのマップ
var visited = make(map[string]bool)

// 訪問済みURLの排他制御用
var mu sync.Mutex

// ゴルーチンの制御用
var workerLimit = make(chan struct{}, 10)

// データ構造体
type Data struct {
    URL  string `json:"url"`
    HTML string `json:"html"`
}
type Config struct {
    BaseURL  string `json:"base_url"`
    Depth    int    `json:"max_depth"`
    Time     time.Duration   `json:"time"`
}


// 引数の初期化
func init() {
    flag.StringVar(&startURL, "startURL", "https://note.com", "ベースポイントとなるURL")
    flag.IntVar(&maxDepth, "maxDepth", 2, "ベースURLを基準とした探索する最大の深さ")
    flag.StringVar(&outputDirPath, "outputDirPath", "./data", "保存先のディクレトリのパス")
    flag.Parse()
}

// 指定したURLのHTMLを取得する
func GetHTML(url string) (string, error) {
	client := &http.Client{
		Timeout: 10 * time.Second, // タイムアウトを10秒に設定
	}

	res, err := client.Get(url)
	if err != nil {
		fmt.Println("Error fetching URL:", err)
		return "", err
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		fmt.Println("Error reading response body:", err)
		return "", err
	}

	return string(body), nil
}

// 現在時刻を取得
func getNowTime() string {
	now := time.Now()

	// 年月日と時間をフォーマット
	formatted := fmt.Sprintf("%d-%02d%02d-%02d%02d",
		now.Year(),         // 年
		int(now.Month()),   // 月
		now.Day(),          // 日
		now.Hour(),         // 時
		now.Minute(),       // 分
	)
    return formatted
}

func createBaseDir() string {
    baseDirPath := filepath.Join(outputDirPath, getNowTime())
    if err := os.Mkdir(baseDirPath, 0755); err != nil {
		log.Fatal(err)
	}
    return baseDirPath
}

// jsonlファイルを作成
func createFile() (*os.File, *os.File, error) {
    baseDirPath := createBaseDir()

    outputFilePath := filepath.Join(baseDirPath, "output.jsonl")
    outputFile, err := os.Create(outputFilePath)
    if err != nil {
        return nil, nil, fmt.Errorf("failed to create output file: %w", err)
    }

    configFilePath := filepath.Join(baseDirPath, "config.jsonl")
    configFile, err := os.Create(configFilePath)
    if err != nil {
        return nil, nil, fmt.Errorf("failed to create config file: %w", err)
    }

    return outputFile, configFile, nil
}

// jsonlファイルにデータを追加
func addJSONL[R Data|Config](record R, file *os.File) error {
    // エンコード
    jsonData, err := json.Marshal(record)
    if err != nil {
        return fmt.Errorf("failed to marshal: %w", err)
    }

    // 書き込み
    if _, err = file.Write(jsonData); err != nil {
        return fmt.Errorf("failed to write %w", err)
    }

    // 行の終わりに改行を追加
    if _, err = file.WriteString("\n"); err != nil {
        return fmt.Errorf("failed to write: %w", err)
    }

    return nil
}

// URLを正規化する
func normalizeURL(baseURL, href string) string {
    // ベースURLをパース
	base, err := url.Parse(baseURL)
    if err != nil {
        fmt.Printf("Invalid base URL: %s\n", baseURL)
        return ""
    }

    // hrefをパース
    ref, err := url.Parse(href)
    if err != nil {
        fmt.Printf("Invalid base URL: %s\n", href)
        return ""
    }

    // 正規化
    return base.ResolveReference(ref).String()
}

// クローリング関数
func Crawl(url string, depth int, file *os.File, wg *sync.WaitGroup) {
	defer wg.Done() // ゴルーチン終了時にカウントを減らす

    // ゴルーチンの制御
    workerLimit <- struct{}{}
    defer func() { <-workerLimit }()

	// 深さが上限を超えた場合は終了
	if depth > maxDepth {
		return
	}

	// URLがすでに訪問済みかを確認
	mu.Lock()
	if visited[url] {
		mu.Unlock()
		return
	}
	visited[url] = true // 訪問済みにマーク
	mu.Unlock()

	// HTMLを取得
	html, err := GetHTML(url)
	if err != nil {
		fmt.Printf("Failed to fetch %s: %v\n", url, err)
		return
	}

	// HTMLをパース
	doc, err := htmlquery.Parse(strings.NewReader(html))
	if err != nil {
		fmt.Printf("Error parsing HTML at %s: %v\n", url, err)
		return
	}

	// htmlを取得
	htmlText := htmlquery.OutputHTML(doc, true)

    // jsonlにデータを追加
    record := Data{URL: url, HTML: htmlText}
    if err := addJSONL(record, file); err != nil {
        fmt.Printf("Error writing to jsonl file: %v\n", err)
    }
	
	// ページ内のリンクを取得
	list := htmlquery.Find(doc, "//a/@href")

	// 次の階層のURLをクローリング
	for _, val := range list {
		href := htmlquery.InnerText(val)

		// URLを正規化
		nextURL := normalizeURL(url, href)

		// startURLの配下かをチェック
		match, _ := regexp.MatchString("^" + regexp.QuoteMeta(startURL), nextURL)
		if !match {
			// fmt.Printf("Skipping; %s (not under %s)\n", nextURL, startURL)
			continue
		}

		// リンクを1秒ごとにスリープ
		time.Sleep(1 * time.Second)

		// 新しいURLを非同期でクローリング
		wg.Add(1)
		go Crawl(nextURL, depth+1, file, wg)
	}

	fmt.Printf("Visited: %s (Depth: %d)\n", url, depth)
}

// メイン関数
func main() {
	startTime := time.Now()

	// 保存ファイル、設定ファイルを作成
	outputFile, configFile, err := createFile()
    if err != nil {
        log.Fatalf("Error create files: %w\n", err)
    }

    // WaitGroupを追加
	var wg sync.WaitGroup
	wg.Add(1)

	go Crawl(startURL, 0, outputFile, &wg)
	wg.Wait() // すべてのクローリングが完了するまで待機

	elapsedTime := time.Since(startTime)

    // configファイルを追加
	record := Config{BaseURL: startURL, Depth: maxDepth, Time: elapsedTime}
    addJSONL(record, configFile)
}
