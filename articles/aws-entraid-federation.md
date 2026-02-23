---
title: "AWSからセキュアにAzureOpenAIにリクエストする"
emoji: "😊"
type: "tech" # tech: 技術記事 / idea: アイデア
topics: ["aws", "iam", "azure", "entraid"]
published: false
---

## 概要

生成AIを利用するサービスをAWSで開発していてGPT系モデルに対応したい場合、エンプラ系だとAzure OpenAIを利用するケースは多いと思います。

ただ、Azureといった外部サービスの認証についてはAPIキーや長寿命なクレデンシャルをローテーションして使う必要があり、あまりセキュアとは言えない状況でした。

そんな折、2025年のre:InventでAWS IAM Outbound Identity Federationが発表され、これまでできなかったサービス間の認証を短命なJWTで実施することが可能になりました。

https://aws.amazon.com/jp/blogs/news/simplify-access-to-external-services-using-aws-iam-outbound-identity-federation/

今回はhonoを用いたAzure OpenAIの呼び出しを例に、AWSとAzure間の認証をセキュアに実施する方法について紹介します。

### 対象読者

- AWSアカウントとAzureアカウントを所持している
- AWSのIAMロールや、Azureのリソースグループの概念について知っている
- AWS CDKの使い方を知っている
  - LambdaやIAMロールの作成に使用
- AWSからAzureへのアクセスにAPIキーや長寿命なクライアントシークレットを使っている

### 検証環境

- M4 MacBook Air (macOS 26.3)
- AWSリージョン ap-northeast-1
- Node.js v24.13.1
- AWS CDK v2.238.0
- hono v4.12.0

### おおまかな流れ

1. AWS IAMのOutbound Identityを有効化（AWSアカウント単位）
2. GetWebIdentityTokenが実行可能なIAMロールを作成 > Lambdaにアタッチ
3. Azure EntraID App registrationを作成 > フェデレーション資格情報に検証先の発行者URLとIAMロールのARNを登録
4. EntraID で作成したアプリケーションに対してAzure OpenAI Serviceの実行権限を付与
5. LambdaでGetWebIdentityTokenから得られたJWTを用いて、EntraID側に一時トークンの発行をリクエスト
6. 一時トークンを利用しAzure OpenAI Serviceにリクエスト

![news-iam-web-identity](/images/aws-entraid-federation/news-2025-iam-web-identity-3-4.png)
参照元: https://aws.amazon.com/jp/blogs/news/simplify-access-to-external-services-using-aws-iam-outbound-identity-federation/


## AWS側の作業

まずはIAM Outbound Identityを有効化（AWSアカウント単位）します。

1. AWSコンソールにログイン
2. 「IAM」> 「アカウント設定」に移動
3. 「アウトバウンド ID フェデレーション」の「有効化」をクリック
4. 「トークン発行者 URL」に記載のURLをメモしておく
例: https://aaaaaaaa-1111-bbbb-2222-cccc3333dddd.tokens.sts.global.api.aws

![IAM Outbound Identity Enabled](/images/aws-entraid-federation/aws-iam.png)

（AWS CLIで実行する場合）

```bash
aws iam enable-outbound-web-identity-federation
```

### CDKリソース作成

以下を参考に`cdk init`コマンドを利用し、プロジェクトを初期化します。

https://hono.dev/docs/getting-started/aws-lambda

```bash
mkdir -p workspace/aws-azure-federation
cd workspace/aws-azure-federation
npx cdk init -l ts
npm i hono
npm i -D esbuild
mkdir lambda
touch lambda/index.ts
```

作成された`lib/aws-azure-federation-stack.ts`に以下を記述します。

:::message alert
作成されるリソースには認証がないため誰でもアクセスできる状態でデプロイされます。
本番環境ではIAM認証やAPI Gatewayで保護してください。
:::

```ts:lib/aws-azure-federation-stack.ts
import * as cdk from "aws-cdk-lib/core";
import { Construct } from "constructs";
import * as iam from "aws-cdk-lib/aws-iam";
import * as lambda from "aws-cdk-lib/aws-lambda";
import { NodejsFunction } from "aws-cdk-lib/aws-lambda-nodejs";

export class AwsAzureFederationStack extends cdk.Stack {
  constructor(scope: Construct, id: string, props?: cdk.StackProps) {
    super(scope, id, props);

    const logGroup = new logs.LogGroup(this, "LogGroup", {
      retention: logs.RetentionDays.ONE_MONTH,
      removalPolicy: cdk.RemovalPolicy.DESTROY,
    });

    const fn = new NodejsFunction(this, "Fn", {
      entry: "lambda/index.ts",
      handler: "handler",
      runtime: lambda.Runtime.NODEJS_24_X,
      timeout: cdk.Duration.minutes(5),
      bundling: {
        minify: true,
      },
      logGroup: logGroup,
    });
    
    const fnUrl = fn.addFunctionUrl({
      authType: lambda.FunctionUrlAuthType.NONE,
    });

    fn.addToRolePolicy(
      new iam.PolicyStatement({
        effect: iam.Effect.ALLOW,
        actions: ["sts:GetWebIdentityToken"],
        resources: ["*"],
        conditions: {
          "ForAnyValue:StringEquals": {
            "sts:IdentityTokenAudience": "api://AzureADTokenExchange",
          },
          NumericLessThanEquals: {
            "sts:DurationSeconds": 300,
          },
        },
      }),
    );
    new cdk.CfnOutput(this, "FnUrl", {
      value: fnUrl.url!,
    });
    new cdk.CfnOutput(this, "FnArn", {
      value: fn.role?.roleArn!,
    });
  }
}
```

`lambda/index.ts`に以下を記述します。

```ts:lambda/index.ts
import { Hono } from 'hono'
import { handle } from 'hono/aws-lambda'

const app = new Hono()

app.get('/', (c) => c.text('Hello World!'))

export const handler = handle(app)
```

`aws login`などを実行しAWSへのクレデンシャルが設定された状態で、`cdk deploy`を実行しリソースをデプロイします。

```bash
$ npx cdk deploy --require-appoval never
...
✨  Deployment time: 1.51s

Outputs:
AwsAzureFederationStack.FnRoleArn = arn:aws:iam::1234567890:role/AwsAzureFederationStack-AwsAzureFederationFunctionServi-hogehoge
AwsAzureFederationStack.FnUrl = https://hogehogefugafuga.lambda-url.ap-northeast-1.on.aws/
```

ブラウザなどでOutputsのURLにアクセスし、Hello World!が表示されることを確認します。

![hello-hono](/images/aws-entraid-federation/hello-world.png)

:::message
LambdaにアタッチされているIAMロールのARNについてメモしておきます。
:::
```
arn:aws:iam::1234567890:role/AwsAzureFederationStack-AwsAzureFederationFunctionServi-hogehoge
```

## Azure側の作業

Azure外部からアクセスさせるため、アプリケーションタイプのサービスプリンシパルを作成します。

https://learn.microsoft.com/ja-jp/entra/identity-platform/app-objects-and-service-principals?tabs=browser

1. Azure portalにアクセスします
2. 「EntraID」> 「アプリの登録」から新規登録
3. 「名前」に任意の値を入力し、その他はデフォルトのまま「登録」

:::message
アプリケーション（クライアント）IDとテナントIDをメモしておきます
:::

![entraid-app](/images/aws-entraid-federation/entraid-app.png)

次に、作成したプリンシパルに対してフェデレーション資格情報を設定し、信頼関係を作成します。

https://learn.microsoft.com/ja-jp/entra/workload-id/workload-identity-federation

https://learn.microsoft.com/ja-jp/entra/workload-id/workload-identity-federation-create-trust-user-assigned-managed-identity?pivots=identity-wif-mi-methods-azp

1. 「すべてのアプリケーション」から先ほど作成したアプリを選択し、「証明書とシークレット」を選択
2. 「フェデレーション資格情報」から「資格情報の追加」を選択
3. 以下の内容で登録

|項目 | 値
| --- | ---
| フェデレーション資格情報のシナリオ | その他の発行者
| 発行者 | AWSアカウントの「トークン発行者URL」
| 種類 | 明示的なサブジェクト識別子にチェック
| 値 | LambdaにアタッチされているIAMロールのARN
| 名前 | 任意の名前
| 説明 | 任意の説明
| 対象ユーザー | デフォルト値(api://AzureADTokenExchange)

:::message alert
現時点では「明示的なサブジェクト識別子」しか利用できないため、ワイルドカードなどが利用できません。
そのため、複数ロールからアクセスしたい場合はロールの数だけ登録が必要です。

なお、1つのアプリケーションに対して設定可能なフェデレーション資格情報は**最大20件**までです。
:::
:::message
ワイルドカードの利用には「柔軟なフェデレーションID資格情報」が必要ですが、現時点（2025/02/23）ではGitHub、GitLab、Terraform Cloudのみがサポートされています
:::

https://learn.microsoft.com/ja-jp/entra/workload-id/workload-identities-set-up-flexible-federated-identity-credential?tabs=azure-portal%2Cgithub

最後に、サービスプリンシパルに対してAzureOpenAIにアクセスするための権限を追加します。
（リソースグループとAzureOpenAIリソースの作成については割愛します）


1. AzureOpenAIリソースの「アクセス制御（IAM）」>「追加」>「ロールの割り当ての追加」
2. 「ロール」>「Cognitive Services OpenAI User」を選択
3. 「メンバー」>「ユーザー、グループ、またはサービスプリンシパル」で先ほど作成したEntraIDのアプリケーションを選択し「レビューと割り当て」を実施

![role-assignment](/images/aws-entraid-federation/role-assignment.png)

:::message
AzureOpenAIのエンドポイントをメモしておきます。
:::

以上でAWSからAzureへのリクエストを行うための準備が完了しました。

## AWS LambdaからAzure OpenAIへのリクエスト

Lambdaから実行するためコードに追記します。

### CDKコードの修正

- AWS SDKをバンドルするように変更
- 先ほどメモしたAzure側のテナントID、アプリケーションID、AzureOpenAIのエンドポイントを設定

```diff ts:lib/aws-azure-federation-stack.ts
import * as cdk from "aws-cdk-lib/core";
import { Construct } from "constructs";
import * as iam from "aws-cdk-lib/aws-iam";
import * as lambda from "aws-cdk-lib/aws-lambda";
import { NodejsFunction } from "aws-cdk-lib/aws-lambda-nodejs";

export class AwsAzureFederationStack extends cdk.Stack {
  constructor(scope: Construct, id: string, props?: cdk.StackProps) {
    super(scope, id, props);

    const logGroup = new logs.LogGroup(this, "LogGroup", {
      retention: logs.RetentionDays.ONE_MONTH,
      removalPolicy: cdk.RemovalPolicy.DESTROY,
    });

    const fn = new NodejsFunction(this, "Fn", {
      entry: "lambda/index.ts",
      handler: "handler",
      runtime: lambda.Runtime.NODEJS_24_X,
      timeout: cdk.Duration.minutes(5),
      bundling: {
        minify: true,
+        bundleAwsSDK: true
      },
      logGroup: logGroup,
+      environment: {
+        TENANT_ID: "{AzureテナントID}",
+        APPLICATION_ID: "{作成したAzureアプリケーションID}",
+        AOI_ENDPOINT: "https://{AzureOpenAIリソース名}.openai.azure.com/openai/v1/",
+      },
    });
    
    // ...snip...
    
    new cdk.CfnOutput(this, "FnArn", {
      value: fn.role?.roleArn!,
    });
  }
}
```

### Lambda関数の実装

必要なパッケージのインストール

```bash
npm install @azure/identity @aws-sdk/client-sts openai
```

以下の流れでAzureOpenAIにリクエストを行います。

1. STSに対してGetWebIdentityTokenの取得
2. EntraIDに対してクレデンシャルのリクエスト
3. 取得したクレデンシャルを用いてAzureOpenAIにリクエスト
 
今回はAzureOpenAIの`v1`エンドポイントを利用します。

https://learn.microsoft.com/ja-jp/azure/ai-foundry/openai/api-version-lifecycle?view=foundry-classic&tabs=python


**1. GetWebIdentityTokenCommandを用いてJWTを取得**
- AudienceはEntraIDで登録した「対象ユーザー」と同じ`api://AzureADTokenExchange`
- DurationSecondsはトークンの有効期限を設定
- SigningAlgorithmは`RS256`
  - AWS側は`ES384`と`RS256`をサポートしているが、EntraIDとしては`RS256, HS256, ES256`をサポートするため、`RS256`を選択
- Azure側のSDKの都合のため、`Promise<string>`を返す

```ts:lambda/index.ts
import { GetWebIdentityTokenCommand, STSClient } from "@aws-sdk/client-sts";

const stsClient = new STSClient();

async function getWebIdentityToken(): Promise<string> {
  const command = new GetWebIdentityTokenCommand({
    Audience: ["api://AzureADTokenExchange"],
    DurationSeconds: 300,
    SigningAlgorithm: "RS256",
  });
  const response = await stsClient.send(command);
  const token = response.WebIdentityToken;
  if (!token) {
    throw new Error("Failed to get web identity token");
  }
  return Promise.resolve(token);
}
```

**2. EntraIDのクレデンシャル取得**

クレデンシャル取得には以下の3点が必要です。
- AzureのテナントID
- 「アプリの登録」で作成したアプリケーションID
- JWTを取得する関数

クレデンシャル情報を用いてAzureOpenAIにリクエストを行うためのトークンを取得します
`getToken`の引数は取得するトークンのスコープを決定するためのものです。

```ts:lambda/index.ts
import { ClientAssertionCredential } from "@azure/identity";

async function getCredential(tenantId: string, applicationId: string) {
  const credential = new ClientAssertionCredential(
    tenantId,
    applicationId,
    getWebIdentityToken,
  );
  return await credential.getToken(
    "https://cognitiveservices.azure.com/.default",
  );
}
```

**3. クレデンシャルを用いてAzureOpenAIにリクエスト**

- 環境変数からテナントID、アプリケーションID、AzureOpenAIのエンドポイントを取得
- AzureOpenAIは`v1`エンドポイントを利用
- 今回は`gpt-5-nano`をデプロイした場合を想定

エンドポイント: `https://{AzureOpenAIのリソース名}.openai.azure.com/v1/`

```ts:lambda/index.ts
import { Hono } from "hono";
import { env } from "hono/adapter";
import OpenAI from "openai";

const api = new Hono();

api.get("/", async(c) => {
  const {
    TENANT_ID: tenantId,
    APPLICATION_ID: applicationId,
    AOI_ENDPOINT: aoiEndpoint,
  } = env<{
    TENANT_ID: string;
    APPLICATION_ID: string;
    AOI_ENDPOINT: string;
  }>(c);
  
  const credential = await getCredential(tenantId, applicationId);
  const openai = new OpenAI({
    baseURL: aoiEndpoint,
    apiKey: credential.token,
  });
  const response = await openai.chat.completions.create({
    model: "gpt-5-nano",
    messages: [{ role: "user", content: "Hello!" }],
  });
})
```

:::details 全体のソースコード

```ts:lambda/index.ts
import { Hono } from "hono";
import { handle } from "hono/aws-lambda";
import { env } from "hono/adapter";
import { GetWebIdentityTokenCommand, STSClient } from "@aws-sdk/client-sts";
import OpenAI from "openai";
import { ClientAssertionCredential } from "@azure/identity";

const stsClient = new STSClient({ region: "us-east-1" });

const app = new Hono();

async function getWebIdentityToken(): Promise<string> {
  const command = new GetWebIdentityTokenCommand({
    Audience: ["api://AzureADTokenExchange"],
    DurationSeconds: 300,
    SigningAlgorithm: "RS256",
  });
  const response = await stsClient.send(command);
  const token = response.WebIdentityToken;
  if (!token) {
    throw new Error("Failed to get web identity token");
  }
  return Promise.resolve(token);
}

async function getAccessToken(tenantId: string, applicationId: string) {
  const credential = new ClientAssertionCredential(
    tenantId,
    applicationId,
    getWebIdentityToken,
  );
  return await credential.getToken(
    "https://cognitiveservices.azure.com/.default",
  );
}

app.get("/", async (c) => {
  const {
    TENANT_ID: tenantId,
    APPLICATION_ID: applicationId,
    AOI_ENDPOINT: aoiEndpoint,
  } = env<{
    TENANT_ID: string;
    APPLICATION_ID: string;
    AOI_ENDPOINT: string;
  }>(c);

  const token = await getAccessToken(tenantId, applicationId);
  const openai = new OpenAI({
    baseURL: aoiEndpoint,
    apiKey: token.token,
  });
  const response = await openai.chat.completions.create({
    model: "gpt-5-nano",
    messages: [{ role: "user", content: "Hello!" }],
  });
  return c.json(response.choices[0].message.content);
});

export const handler = handle(app);
:::

### デプロイ

最後にCDKデプロイを実施し、再度URLにアクセスします。

```bash
npx cdk deploy --require-approval never
```

ブラウザ上にAIによる応答が得られていれば成功です！

![aoi-result](/images/aws-entraid-federation/aoi-result.png)

## お片付け

最後にAWS側のリソースとAzureEntraIDのアプリケーションを削除します。

### AWSリソースの削除

`cdk destroy`でリソースを削除します。

```bash
$ npx cdk destroy
(node:65214) [DEP0169] DeprecationWarning: `url.parse()` behavior is not standardized and prone to errors that have security implications. Use the WHATWG URL API instead. CVEs are not issued for `url.parse()` vulnerabilities.
(Use `node --trace-deprecation ...` to show where the warning was created)
Are you sure you want to delete: AwsAzureFederationStack (y/n) y
AwsAzureFederationStack: destroying... [1/1]

 ✅  AwsAzureFederationStack: destroyed
```

### Azureリソースの削除

1. Azureポータルにアクセス
2. EntraID > アプリの登録 > すべてのアプリケーション
3. 作成したアプリを選択 > 「削除」をクリック

![aoi-destroy](/images/aws-entraid-federation/entraid-destroy.png)

### AzureOpenAIの削除

1. Azureポータルにアクセス
2. 作成したリソースグループに移動
3. 作成したリソースを選択 > 「Foundaryポータルに移動」
4. デプロイしたモデルをすべて削除
5. 元のポータルに戻り、AzureOpenAIリソースを削除

![aoi-destroy](/images/aws-entraid-federation/aoi-destroy.png)

## まとめ

AWS IAM Outbound ID Federationを用いることで、これまでできなかったAWSから外部サービスの呼び出しを短命なJWTで行うことができ、よりセキュアな通信が可能になりました。

また、長期間有効なシークレットのローテーション作業も不要になるため、シークレットの更新し忘れでサービスが動作しなくなるリスクも軽減されます。

ただし、複数のIAMロールからアクセスさせたい場合、現在はワイルドカードなどが利用できないためロール毎にARNを登録する必要がある点に注意が必要です。

## 参考文献
https://aws.amazon.com/jp/blogs/news/simplify-access-to-external-services-using-aws-iam-outbound-identity-federation/
https://learn.microsoft.com/ja-jp/entra/workload-id/workload-identity-federation
