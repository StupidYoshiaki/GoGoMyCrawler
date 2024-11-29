# GoGoMyCrawler
golangで実装したクローラー(未完成)

## 使い方
crawler.goをビルド
```bash
go build src/crawler.go
```
実行(例)
```bash
./crawler -startURL=https://npb.jp/ -maxDepth=2 -outputDirPath=./data
```