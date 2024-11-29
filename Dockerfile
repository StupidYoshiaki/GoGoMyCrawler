# Ubuntuの公式イメージを使用
FROM ubuntu:22.04

# タイムゾーンの設定
RUN ln -sf /usr/share/zoneinfo/Asia/Tokyo /etc/localtime

# 作業ディレクトリを設定
WORKDIR /workspace

# 必要なツールとパッケージをインストール
RUN apt-get update && apt-get install -y \
    golang git tree tmux \
    && apt-get clean

# プロキシ設定（必要なら有効化）
ENV GOPROXY=https://proxy.golang.org,direct

# Go Modules用の初期化
RUN mkdir -p /workspace && go mod init workspace

# プロジェクト全体をコピー
COPY . .

# 依存関係のダウンロード
RUN go mod tidy

# シェルを起動
CMD ["/bin/bash"]




# 以下のコマンドでイメージをビルド: docker build -t go_crawler .
# 以下のコマンドでコンテナを起動: docker run -it --name go_crawler_container -v $(pwd):/workspace go_crawler