---
title: "新卒社員が社内向けAIチャットサービスを構築した話 (前編)"
emoji: "👶"
type: "tech" # tech: 技術記事 / idea: アイデア
topics: [Azure, OpenAI]
published: false
---
# はじめに
先日Azure OpenAI Service のJapan East リージョン提供開始のアナウンスがあり、国内で使える生成AIの関心は日々高まっています。

https://www.publickey1.jp/blog/23/azure_openai_service.html

本記事では社内向けに Azure OpenAI (以下AOAI) を展開するにあたり、やったことや色んなハマりポイントを備忘録としてまとめました。
前後編の構成で、前編は主に作成したアーキテクチャについて、後編ではハマったポイントを中心に書いていきます。

::: message
あくまで社内の環境に合わせて作成したものです。参考程度にしてください。
:::

# 前提

オンプレミスから仮想ネットワークまでの経路は既に閉域網でアクセス可能であることを想定しています。具体的には、ExpressRoute やSite to Site VPN などが用意されており VNet のリソースに閉域網でアクセスできる状態です。
Virtual Network は Hub and Spoke の構成を取っており、リソースを配置するネットワークにはオンプレDNSを利用するためのカスタムDNSを設定しています。

# 最終的なゴールイメージ

Microsoft 公式のサンプルとほぼ同じ構成を目指します。

https://github.com/Azure-Samples/jp-azureopenai-samples/tree/main/6.azureopenai-landing-zone-accelerator

![overview](/images/aoai-internal/overview.png)

- オンプレとAzureはExpressRouteで接続します
- WebUIの提供とアクセスログの監視を行います
- AppService および API Management ではAD認証を行い、利用可能なユーザーを制限します
- Azure内部のリソースは基本的にパブリックアクセスを許可しません
- AppServiceおよびAPI Managementには社内で利用するカスタムドメインを設定します
    - デフォルト(*.azurewebsites.net)だとパブリックに名前解決してしまう設定であったため

# Azure Virtual Network（VNet）の作成

今回は例として 10.0.1.0/24 の CIDR を用意し適当なサブネットを作成します。()の中はサブネットの範囲です。
簡単のためVNetは一つしか作成しませんが、必要に応じてVNetを分けるなどしてください。

- PrivateEndpoint 作成用のサブネット (エンドポイントを置く数だけ)
- AppService VNet統合専用 (/26 ~ /28)
- (Optional) DNS Private Resolver 受信エンドポイント専用

::: message
下2つはリソース専用のサブネットになります
:::

DNS Private Resolver はオンプレ側から名前解決できる場合不要になります。これを使う場合、利用者PCの優先DNSを変更するか、オンプレ側のDNSサーバーにNSレコードを追加する必要があります。
今回はカスタムドメインを利用し、プライベートエンドポイントのNICに割り当てられたIPアドレスをAレコードとしてオンプレDNSに登録しています。

# Azure OpenAI （AOAI） の作成

AOAI の利用にはOrganizationに所属しているアカウントで申請が必要です。

https://learn.microsoft.com/ja-jp/azure/ai-services/openai/overview
ネットワークタブから許可するアクセス元のCIDRを登録する、もしくは後述するプライベートエンドポイントを利用します。また、必要に応じて IAM からアクセスを制限します。
今回は API Management からのアクセスに限定し、API Management のパブリックIPを許可します。プライベートエンドポイントを利用しない理由については API Management の作成時に説明します。

::: message alert
すべてのネットワークを許可すると、どこからでもアクセス可能な状態になってしまいます
:::
作成後は自分たちの利用したいモデル（GPT3.5など）を Azure OpenAI Studio からデプロイしてください。

# API Management の作成

API Management は AWS でいう API Gateway と同様の役割を担います。
閉域化には二つのアプローチがあり排他的です。

- プライベートエンドポイントを作成し、インバウンドポリシーによってアクセスを制限する
- VNet の内部にリソースを配置し、外部から完全にアクセスできないようにする

後者の方が理想的に見えますが、NSGの設定が煩雑になること、Developer か Premium プランのみ機能が利用可能であることから今回は見送りました。
特に、Premium プランは $2,795.17/month と非常に高額であるため利用を断念しました。

(参考) https://azure.microsoft.com/ja-jp/pricing/details/api-management/#pricing

前者を利用する場合、インバウンドは VNet から可能になりますが、アウトバウンドはパブリックになるため AOAI のプライベートエンドポイントに直接アクセスできません。そのため、AOAI が許可するアクセス元として API Management のパブリックIPを許可する必要があります。
~~どうみてもイケてないので、~~ 他にいい方法がある場合は是非コメントをお待ちしております。

## プライベートエンドポイントを設置する

API Management の ネットワークタブからプライベートエンドポイントを有効化します。
エンドポイント用に作成したサブネットに配置してください。

![apim_pep](/images/aoai-internal/apim_pep.png)

エンドポイントのIPは自動で振られるので、適当に10.0.1.10 であったとします。

## API Management のバックエンドに AOAI を置く

API Management で AOAI を扱う方法は先人たちがまとめてくださっているので、そちらを参照すれば問題ないかと思います。

https://level69.net/archives/33697

例として今回は `/openai` をサフィックスとして利用します。

## AOAIの受信ファイアウォールを設定する

API Management からのみ受信を許可するため、AOAIのネットワークタブから「受信を許可するIPアドレス」として API Management のパブリックIPを指定します。
API Management をVNet内に配置できればプライベートエンドポイントに対してアクセスできましたが、今回は内部に設置しないためこのような方法を取ります。

## ログの設定

API Management の診断設定から、LogAnalyticsやBlobストレージへの保存を有効にします。
ログを取りたいAPIの設定から Azure Monitorを有効にし、適当に設定してください。
設定の仕方は以下の記事を参考にしました。

https://ayuina.github.io/ainaba-csa-blog/monitoring-api-management/

## Inboud Policy を設定する

受信した Body に有効な JWT (アクセストークン) が含まれているか検証します。
Azure AD を利用する場合、インバウンドポリシーに `<validate-azure-ad-token>` を利用することで簡単に検証が可能です。以下は必要最低限のスニペットです。

https://learn.microsoft.com/ja-jp/azure/api-management/validate-azure-ad-token-policy#examples

```xml
<policies>
    <inbound>
        <validate-azure-ad-token tenant-id="{{aad-tenant-id}}">
            <client-application-ids>
                <application-id>{{aad-client-application-id}}</application-id>
            </client-application-ids>
        </validate-azure-ad-token>
    </inbound>
    ...
</policies>
```


`{{aad-client-application-id}}` には登録したアプリケーションクライアントIDを入れます。今回は AppService のアプリケーションIDを入れます。`<client-application-ids>` は子として複数のアプリケーションを持てるため、AppService とAzure Functions のように複数のアプリケーションを検証できます。

プライベートエンドポイントからの受信だけを許可することで擬似的にパブリックアクセスを禁止することができます。以下はサンプルですが、必要に応じてCORSやIPアドレス制限などを設けてください。

https://learn.microsoft.com/en-us/azure/api-management/api-management-policy-expressions

```xml
<policies>
    <inbound>
        <!-- AADから発行されるJWTの検証 -->
        <validate-azure-ad-token tenant-id="{{aad-tenant-id}}">
            <client-application-ids>
                <application-id>{{aad-client-application-id}}</application-id>
            </client-application-ids>
        </validate-azure-ad-token>
        <choose>
            <when condition="@(context.Request.PrivateEndpointConnection == null">
                <!-- プライベートエンドポイント以外からのアクセスを拒否 -->
                <return-response>
                    <set-status code="403" reason="Forbidden" />
                    <set-body>
                        {
                            "error": {
                                "message": "Access Forbidden. Please access from Private endpoint."
                            }
                        }
                    </set-body>
                </return-response>
            </when>
        </choose>
    </inbound>
    ...
</policies>
```


## カスタムドメインの設定

API Management でカスタムドメインを利用する場合、TXTレコードによるドメインの検証か適当な証明書が必要です。今回はワイルドカード証明書を設置しました。
今回は例として `api.contoso.com` というドメインを設定します。

カスタムドメインを利用する理由として、

- 社内にAPIを提供するため、わかりやすい名前をつけたかった
- VNetのカスタムDNSに社内のDNSを見に行くような設定をしており、PrivateDNSZone で名前解決ができなかった

プライベートエンドポイントを作成する場合、作成したVNetにPrivateDNSZoneと呼ばれる名前空間が利用でき簡単にエンドポイントに対して簡単に名前解決が可能になるのですが、カスタムDNSを設定している場合そちらが優先されてしまい結果的にパブリックIPで名前解決がされてしまいました。

オンプレDNSにConditionalForwarderを設定すればPrivateDNSZoneで名前解決してくれるそうですが、そんな権限は無いのでカスタムドメインとプライベートエンドポイントのIPをAレコードで登録申請し、オンプレDNSに問い合わせた結果がプライベートエンドポイントに向くようにしました。

:::details 条件付きフォワーダーを利用する場合のイメージ
![custom-dns-express-route-expanded](https://learn.microsoft.com/ja-jp/azure/machine-learning/media/how-to-custom-dns/custom-dns-express-route-expanded.png?view=azureml-api-2#lightbox)
(参考) https://learn.microsoft.com/ja-jp/azure/machine-learning/how-to-custom-dns?view=azureml-api-2&tabs=azure-cli#example-custom-dns-server-hosted-on-premises
:::

カスタムドメインを設定したのでAPIを叩く時は `api.contoso.com/openai` になります。例えば
```
https://api.contoso.com/openai/deployments/{model_name}/chat/completions?api-version=2023-07-01-preview
```
みたいなURLを指定します。

## 構成図

ここまでで、おおよそ以下の図の通りになります。
(ログの部分は省略してます)

![step1](/images/aoai-internal/step1.png)

API Managementにアクセスできるのは VNet 経由だけであり、AOAI にアクセスできるのはAPI Management のみになります。
また、API ManagementのインバウンドポリシーにJWTの検証を行うことで、AD認証された特定のユーザーのみがAPI Managementを叩くことができます。

# AppService の作成
AppService ではユーザーがアクセスするWebサイトを提供します。
AppServiceは AppServicePlan(ASP) の上に作成するため、事前にASPを作成してください。

UIを提供するアプリケーション部分はインフラ構築とは直接関係ないので、以下の記事通り適当なアプリケーションをデプロイしてください。

https://ks6088ts.github.io/blog/fork-azure-openai-playground/

注意点として、世の中のChatGPTライクなUIを提供するアプリケーションはOpenAI API を前提としたものが多いので、API ManagementのエンドポイントにリクエストされるようにURLを書き換える必要があります。大抵の場合、環境変数などで変更できるとは思います。

## VNetの設定
AppService を直接プライベートサブネットに配置することはできません。受信と送信それぞれに設定が必要になります。

- (受信) プライベートエンドポイントを作成し VNet 側から受信できるようにする
    - パブリックアクセスを禁止する
- (送信) VNet 統合により送信を VNet に向ける

https://learn.microsoft.com/ja-jp/azure/private-link/private-endpoint-overview

プライベートエンドポイントとは VNet 内のサブネットに対して受信専用のエンドポイントを作成する機能です。これによって VNet 内部から AppService にアクセスできます。**パブリックアクセスの禁止**とプライベートエンドポイントの組み合わせにより、イントラネットからのアクセスに限定することができます。

### プライベートエンドポイントネットワークポリシー
プライベートエンドポイントに対してアクセスする場合、私の環境ではエンドポイントを設置しているサブネットに 「プライベートエンドポイントネットワークポリシー」 を設定する必要がありました。
ルートテーブルとしてオンプレのIPからの送受信を許可するルーティングを行っていたのですが、プライベートエンドポイントに対して有効になっていない？ため追加でポリシーの設定が必要でした。(これが必要であることに気づくまで3日間かかりました...)

https://learn.microsoft.com/ja-jp/azure/private-link/disable-private-endpoint-network-policy?tabs=network-policy-portal

### VNet 統合
AppService から VNet 内部のリソースにアクセスするためには VNet統合 を有効にします。これにより Outbound のネットワークを VNet 内部に閉じることができます。

::: message
VNet統合 で指定するサブネットは空である必要があります
VNet統合で必要なサブネットの最小容量は `/28` ですが、不要なエラーを避けるために `/26` を指定するのが無難です
:::

https://learn.microsoft.com/ja-jp/azure/app-service/overview-vnet-integration

## Entra ID による認証

AppService では Entra ID (慣れていないので以降は Azure ADとして表現します) による認証を手軽に行うことができます。「認証」タブからIDプロバイダーを追加するだけで利用できます。

![easy_auth](/images/aoai-internal/easy_auth.png)

ADに登録された特定のユーザーだけを許可したい場合があると思います。エンタープライズアプリケーションからアクセス可能なグループやユーザーを指定することができます。

https://learn.microsoft.com/ja-jp/azure/active-directory/develop/howto-restrict-your-app-to-a-set-of-users

## カスタムドメインの設定

AppService にカスタムドメインを設定する場合 API Management と異なりTXTレコードの検証が必須です。このTXTレコードはパブリックインターネットから解決できる必要があります。
検証が済めば、証明書を登録してバインドして完了です。
今回は `web.contoso.com` として設定し、オンプレDNSにプライベートエンドポイントのIPとドメインをAレコードで登録申請しました。

### AzureADログインリダイレクト先の変更

カスタムドメインを利用する場合、ADログイン後のリダイレクトURIが既定のドメインと異なるため追加しないとCORSエラーが発生します。
AzureAD → アプリの登録 → 認証 からリダイレクトURIを追加します。
今回であれば以下のようなURIを追加します。
`https://web.contoso.com/.auth/login/aad/callback`

## 構成図
ざっくりと表すと以下のようになります。
流れとしては、

1. `web.contoso.com`がオンプレDNSサーバーから名前解決される(10.0.1.11)
1. ユーザーが社内LANからアクセスする
1. AzureADによるログインとリダイレクト → WebUI表示
1. WebUIに適当なプロンプトを送信
1. AppServiceは`api.contoso.com`をカスタム(オンプレ)DNSから名前解決(10.0.1.10)
1. `api.contoso.com/openai` に対して送信し、レスポンスを得る

![step2](/images/aoai-internal/step2.png)

以上が私が作成した社内向けAIチャットサービスの概要となります。

# おわりに

初めてクラウドをまともに触ってみてネットワークやAzure独特の考え方などを学ぶ良い機会になりました。特に環境が多少特殊であることや、なぜ繋がらないのか原因を突き止めるための手段が分からないことが多く、何度もVMを立てるなどして苦労しました。
現在はこれを更に拡張して社内文書検索の対応やIaCによる資産化などを目指しています。
後編では、作成時にハマったポイントや対応策などを述べていきます。

# 参考

https://dev.classmethod.jp/articles/azure-openai-chatbot-in-closed-network/
https://blog.aelterman.com/2022/01/10/azure-app-service-using-a-custom-domain-name-in-a-private-namespace/
https://christina04.hatenablog.com/entry/2016/06/07/123000
