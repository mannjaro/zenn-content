---
title: "新卒社員が Azure OpenAI Service を閉域網で展開した話"
emoji: "👶"
type: "tech" # tech: 技術記事 / idea: アイデア
topics: ["Azure", "OpenAI"]
published: false
---
# はじめに
先日Azure OpenAI Service のJapan East リージョン提供開始のアナウンスがあり、国内で使える生成AIの関心は日々高まっています。

https://www.publickey1.jp/blog/23/azure_openai_service.html

本記事では閉域網に Azure OpenAI Service を展開するにあたり、やったことや色んなハマりポイントを備忘録としてまとめました。Azure を含めクラウドサービスの利用は初めてであり、理解の至らない箇所が多々あるかと思います。是非、愛のあるコメントをお待ちしています。

# 前提

オンプレミスからAzure仮想ネットワークまでの経路は既に閉域網でアクセス可能であることを想定しています。具体的には、ExpressRoute やSite to Site VPN などが用意されており VNet のリソースに閉域網でアクセスできる状態です。また、ネットワークトポロジーとして Hub and Spoke を採用しています。

https://learn.microsoft.com/ja-jp/azure/architecture/reference-architectures/hybrid-networking/hub-spoke?tabs=cli

Azure OpenAI Service を利用するためには申請が必要です。申請には Organization に所属しているサブスクリプションが必要になります。 

https://learn.microsoft.com/ja-jp/azure/ai-services/openai/overview

**※ 記事を利用したことにより被るいかなる損害についても責任を負うものではありません。**

# 最終的なゴールイメージ

Microsoft 公式のサンプルとほぼ同じ構成を目指します。

https://github.com/Azure-Samples/jp-azureopenai-samples/tree/main/6.azureopenai-landing-zone-accelerator

AppService による ChatGPT ライクな WebUI と、API Management によるアクセスログの監視を行います。

## Azure Virtual Network（VNet）の作成

VNet は既に以下のような構成が取られていました。

![]()

特に注目するべき点は、カスタムDNSが設定されていることだと思います。イントラネット向けに DNS サーバーが設置されているのは珍しいことでは無く、担当した環境では Azure 側からオンプレに名前解決できるような仕掛けがされていました。
これのおかげでめちゃくちゃハマったので後ほど解説します。

今回は VNet として 10.0.1.0/24 の CIDR が用意されているという前提で進めます。
最初はサブネットとして 10.0.1.0/24 が使われていたため、次のように分割を行いました。

- PrivateEndpoint 作成用のサブネット
- AppService VNet統合専用
- DNS Private Resolver 受信エンドポイント専用

::: message
下2つはサブネット内に何も置いてはいけない 専用 のサブネットになります。
:::

ややこしくなりますが、DNS Private Resolver は後ほどイントラネット向けにDNSを登録することで不要になります。

## Azure OpenAI （AOAI） の作成

ネットワークタブから許可するアクセス元のCIDRを登録する、もしくは後述するプライベートエンドポイントを利用します。また、必要に応じて IAM からアクセスを制限します。
今回は API Management からのアクセスに限定し、API Management のパブリックIPを許可します。プライベートエンドポイントを利用しない理由については API Management の作成時に説明します。

::: message alert
すべてのネットワークを許可すると、どこからでもアクセス可能な状態になってしまいます
:::
AOAI 作成後は自分たちの利用したいモデル（GPT3.5など）を Azure OpenAI Studio からデプロイしてください。

## AppService の作成

AppService ではリソースを直接プライベートサブネットに配置することはできません。そのため受信と送信それぞれに設定が必要になります。

- プライベートエンドポイントを作成し VNet から受信する
- VNet 統合により送信を VNet に向ける

プライベートエンドポイントとは VNet 内のサブネットに対して受信専用のエンドポイントを作成する機能です。これによって VNet 内部から AppService にアクセスできます。パブリックアクセスの禁止とプライベートエンドポイントの組み合わせにより、イントラネットからのアクセスに限定することができます。

https://learn.microsoft.com/ja-jp/azure/private-link/private-endpoint-overview

逆に AppService から VNet 内部のリソースにアクセスするためには VNet統合 を有効にします。これにより Outbound のネットワークを VNet 内部に閉じることができます。

::: message
VNet統合 で指定するサブネットは空である必要があります
VNet統合で必要なサブネットの最小容量は `/28` ですが、不要なエラーを避けるために `/26` を指定するのが無難です
:::

https://learn.microsoft.com/ja-jp/azure/app-service/overview-vnet-integration

### Entra ID による認証

AppService では Entra ID (慣れていないので以降は Azure ADとして表現します) による認証を手軽に行うことができます。「認証」タブからIDプロバイダーを追加するだけで利用できます。

https://learn.microsoft.com/ja-jp/azure/app-service/configure-authentication-provider-aad?tabs=workforce-tenant

ADに登録された特定のユーザーだけを許可したい場合があると思います。エンタープライズアプリケーションからアクセス可能なグループやユーザーを指定することができます。

https://learn.microsoft.com/ja-jp/azure/active-directory/develop/howto-restrict-your-app-to-a-set-of-users

## API Management の作成

API Management は AWS でいう API Gateway と同様の役割を担います。閉域化には二つのアプローチがあり排他の関係にあります。

- プライベートエンドポイントを作成し、インバウンドポリシーによってアクセスを制限する
- VNet の内部にリソースを配置し、外部から完全にアクセスできないようにする

後者の方が理想的に見えますが、NSGの設定が煩雑になること、Developer か Premium プランのみ機能が利用可能であることから今回は見送りました。
特に、Premium プランは $2,795.17/month と非常に高額であるため利用を断念しました。

https://azure.microsoft.com/ja-jp/pricing/details/api-management/#pricing

前者を利用する場合、インバウンドは VNet から可能になりますが、アウトバウンドはパブリックになるため AOAI のプライベートエンドポイントに直接アクセスできません。そのため、AOAI が許可するアクセス元として API Management のパブリックIPを許可する必要があります。
~~どうみてもイケてないので、~~ 他にいい方法がある場合は是非コメントをお待ちしております。

API Management で AOAI を扱う方法は先人たちがまとめてくださっているので、そちらを参照すれば問題ないかと思います。

https://zenn.dev/microsoft/articles/azure-openai-nocode-logging

https://level69.net/archives/33697

### Inboud Policy を設定する

API Management には AppService のようにアプリケーションを登録して認証を行うことはできません。あくまで、受信した Body に有効な JWT が含まれているか検証するのみになります。
Azure AD を利用する場合、インバウンドポリシーに `<validate-azure-ad-token>` を利用することで簡単に検証が可能です。以下は必要最低限のスニペットです。

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

`{{aad-client-application-id}}` には登録したアプリケーションクライアントIDを入れます。今回は AppService のアプリケーションIDを入れます。
また、`<client-application-ids>` は子として複数のアプリケーションを持てるため、AppService とAzure Functions のように複数のアプリケーションを検証できます。

https://learn.microsoft.com/en-us/azure/api-management/validate-azure-ad-token-policy

また、プライベートエンドポイントからの受信だけを許可することで擬似的にパブリックアクセスを禁止することができます。以下はサンプルですが、必要に応じてIPアドレス制限などを設けてください。

```xml
<policies>
    <inbound>
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

# ハマったポイント

## AppServiceのプライベートエンドポイントにアクセスできない

### tl;dr

ユーザー定義ルートを作成し、「プライベートエンドポイントのネットワークポリシー」でテーブルルーティングを有効にする

https://learn.microsoft.com/ja-jp/azure/private-link/disable-private-endpoint-network-policy?tabs=network-policy-portal

### 試してみたこと

一番最初にハマったポイントです。同一サブネットに配置したテスト用VMにはSSHでアクセスできるのに、同じサブネットにあるエンドポイントから全くレスポンスが得られませんでした。
名前解決もできて VNet に到達しているにも関わらずアクセスできず、原因が全く不明でした。

## プライベートエンドポイントの名前解決ができない

## API Management が内部化できない

## AppServiceの認証後リダイレクトURLを変更したい

## AppServiceでカスタムドメインを利用したい

# おわりに
# 参考
