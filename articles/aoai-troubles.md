---
title: "新卒社員が社内向けAIチャットサービスを構築した話 (後編)"
emoji: "⛳"
type: "tech" # tech: 技術記事 / idea: アイデア
topics: [Azure, OpenAI]
published: false
---

# はじめに

# ハマったポイント

## AppServiceのプライベートエンドポイントにアクセスできない

### tl;dr

ユーザー定義ルートを作成し、「プライベートエンドポイントのネットワークポリシー」でテーブルルーティングを有効にする

https://learn.microsoft.com/ja-jp/azure/private-link/disable-private-endpoint-network-policy?tabs=network-policy-portal

### 試してみたこと
同一サブネットに配置したテスト用VMにはSSHでアクセスできるのに、同じサブネットにあるエンドポイントから全くレスポンスが得られませんでした。
名前解決もできて VNet に到達しているにも関わらずアクセスできず、原因が全く不明でした。

## プライベートエンドポイントの名前解決ができない

## API Management が内部化できない

## AppServiceの認証後リダイレクトURLを変更したい

## AppServiceでカスタムドメインを利用したい

