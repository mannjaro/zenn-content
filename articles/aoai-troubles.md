---
title: "新卒社員が社内向けAIチャットサービスを構築した話 (後編)"
emoji: "🕳️"
type: "tech" # tech: 技術記事 / idea: アイデア
topics: [Azure, OpenAI]
published: true
---

# はじめに

閉域網でAppService,API Management, AOAIを構築するときに遭遇したトラブルをまとめます。
アーキテクチャは前編を参照してください。

https://zenn.dev/kris/articles/aoai-internal

構築する上で遭遇したトラブルなどについてまとめます。
人によっては当然だろうと思うところもあるかもしれませんが、初めて触るものなのでまとめておきます。

## AppServiceにアクセスできない

tl;dr
ユーザー定義ルートを作成し、「プライベートエンドポイントのネットワークポリシー」でテーブルルーティングを有効にする。

https://learn.microsoft.com/ja-jp/azure/private-link/disable-private-endpoint-network-policy?tabs=network-policy-portal

閉域化に伴いアクセスをプライベートエンドポイント経由に限定したところアクセスができなくなりました。同一VNet内に配置したVMにはアクセスできており、ネットワークの問題ではないと考えられました。

AppServiceのネットワークポリシーを確認すると、デフォルトでは「プライベートエンドポイントのネットワークポリシー」が無効になっていました。これを有効にすることでアクセスできるようになりました。

## プライベートエンドポイントの名前解決ができない

tl;dr
VNetでカスタムDNSが有効になっており社内のDNSサーバーから解決しようとしていた。
プライベートエンドポイントのNICをAレコードとして登録する。

VNetにカスタムDNSを設定している場合、AzureDNSよりも優先されてしまうためDNS Private Zoneを登録していても名前解決ができませんでした。
カスタムDNSを設定しているVNetからプライベートエンドポイントにアクセスする場合、DNSフォワーダーや条件付きフォワーダーなどを設置するかカスタムDNSに直接登録することで解決できます。
社内DNSの設定を変えるほどの権限は無い(面倒臭い)ので、プライベートエンドポイントのNICをAレコードとして登録しました。
https://cloudsteady.jp/2021/04/09/37510/

## AppServiceにカスタムドメインを登録する

tl;dr
AppServiceの「カスタムドメイン」から認証用のTXTレコードをパブリックDNSに登録する。

社内でしか利用しないサービスでしたが、勝手にカスタムドメインを設定できません。AレコードやCNAMEレコードは社内向けのDNSサーバーに登録しますが、別途TXTレコードをパブリックDNSに登録しAzure側が認証できるようにする必要があります。
カスタムDNSを利用していても関係なく、パブリックDNSに登録する必要があります。

https://blog.aelterman.com/2022/01/10/azure-app-service-using-a-custom-domain-name-in-a-private-namespace/

> When validating the custom domain using DNS, the Azure infrastructure does not use your custom DNS servers. This is true even if VNet integration is enabled.

## .pfx証明書をAppServiceに登録する

tl;dr
opensslコマンドに `-legacy` オプションを付ける。

AppServiceに証明書を登録する際、.pfx形式のファイルをアップロードできます。
社内で証明書を発行し.pfxファイルを作成しましたが、以下のエラーが発生しアップロードに失敗しました。
> At least one certificate is not valid (Certificate failed validation because it could not be loaded.)

秘密鍵の暗号化アルゴリズムがTripleDESでない場合、これらが発生するそうです。
しかし、DESは今後廃止される流れにあり、TripleDESはopensslコマンドではデフォルトで無効になっています。

秘密鍵はaes256で作成していたため、暫定的にopensslコマンドに `-legacy` オプションを付けて.pfxファイルを作成しました。(これで良いのか？)

https://stackoverflow.com/questions/73001634/azure-app-service-unable-to-validate-pfx-file-certificate-failed-validation-be

# まとめ（ポエム）
VNetにカスタムDNSを設定していたり、閉域アクセスに伴うトラブルが多かったです。
クラウドサービスやネットワークに詳しくなかったため苦労しました。特に問題の切り分けが難しく、Azure固有の問題なのか社内の問題なのか判断するのが大変でした。
パケットの到達性を確認するためにWiresharkでパケットキャプチャしたり、AppServiceのログを見たり、VMを立ててBastion経由で確認したりとあの手この手で確認しました。もうちょっと原因を切り分ける能力を身につけたいですね。
(会社はAWSがメインなのでAzureに詳しい人が少なくて辛かった...)