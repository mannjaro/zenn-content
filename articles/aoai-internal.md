---
title: "Azure OpenAI Service を閉域網で展開する"
emoji: "🏢"
type: "tech" # tech: 技術記事 / idea: アイデア
topics: ["Azure", "OpenAI"]
published: false
---
# はじめに
先日Azure OpenAI Service のJapan East リージョン提供開始のアナウンスがあり、日本国内で使える生成AIの関心は日々高まっています。

https://www.publickey1.jp/blog/23/azure_openai_service.html

本記事では閉域網にAzure OpenAI Service を展開するにあたり、色んなハマりポイントを備忘録としてまとめました。

**※ 記事を利用したことにより被るいかなる損害についても責任を負うものではありません。**

# 前提

オンプレミスからAzure仮想ネットワークまでの経路は既に閉域網でアクセス可能であることを想定しています。具体的には、ExpressRoute やSite to Site VPN などが用意されており VNet のリソースに閉域網でアクセスできる状態です。また、ネットワークトポロジーとして Hub and Spoke を採用しています。


https://learn.microsoft.com/ja-jp/azure/architecture/reference-architectures/hybrid-networking/hub-spoke?tabs=cli

Azure OpenAI Service を利用するためには申請が必要です。申請には Organization に所属しているサブスクリプションが必要になります。 

https://learn.microsoft.com/ja-jp/azure/ai-services/openai/overview

# 最終的なゴールイメージ
Microsoft 公式のサンプルとほぼ同じ構成を目指します。

https://github.com/Azure-Samples/jp-azureopenai-samples/tree/main/6.azureopenai-landing-zone-accelerator

AppService による ChatGPT ライクな WebUI と、API Management によるアクセスログの監視を行います。

## Azure OpenAI （AOAI） の作成

特に難しいポイントは無いです。
ネットワークタブから許可するアクセス元のCIDRを登録する、もしくは後述するプライベートエンドポイントを利用します。また、必要に応じて IAM からアクセスを制限します。
今回は API Management からのアクセスに限定し、API Management のパブリックIPを許可します。プライベートエンドポイントを利用しない理由については API Management の作成で説明します。

::: message alert
すべてのネットワークを許可すると、どこからでもアクセス可能な状態になってしまいます
:::
AOAI 作成後は自分たちの利用したいモデル（GPT3.5など）を Azure OpenAI Studio からデプロイしてください。

## AppService の作成

最低限必要なのは以下の3点です。

- プライベートエンドポイントを作成する
- パブリックアクセスを許可しない
- AD認証を有効にし、エンタープライズアプリケーションから「割り当てが必要ですか？」を有効にする

プライベートエンドポイントとは VNet 内のサブネットに対して受信専用のエンドポイントを作成する機能です。これによって VNet 内部から AppService にアクセスできます。

https://learn.microsoft.com/ja-jp/azure/private-link/private-endpoint-overview

逆に AppService から VNet 内部のリソースにアクセスするためには VNet統合 を有効にします。これにより、Inbound 、Outbound のネットワークを VNet 内部に閉じることができます。

https://learn.microsoft.com/ja-jp/azure/app-service/overview-vnet-integration


# おわりに
# 参考
