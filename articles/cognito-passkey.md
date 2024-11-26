---
title: "AWS Cognito の Passkey を試す"
emoji: "🔐"
type: "tech" # tech: 技術記事 / idea: アイデア
topics: [aws, cognito]
published: false
---

# はじめに

11/22にCognitoの大幅アップデートが入り、その中の一つにPasskeyのサポートが追加されました。Passkeyの実装に詳しくなくても、簡単に認証ができたのでその方法についてまとめます。

https://aws.amazon.com/jp/blogs/aws/improve-your-app-authentication-workflow-with-new-amazon-cognito-features/

https://docs.aws.amazon.com/cognito/latest/developerguide/amazon-cognito-user-pools-authentication-flow-methods.html#amazon-cognito-user-pools-authentication-flow-methods-passkey


:::message
Passkeyの仕組みやCognitoについての解説はしません
:::

## 前提条件

- AWS アカウント発行済み
- Cognitoについての基礎知識
- Webフロントエンドについての基礎知識

# Cognito UserPoolの作成

まずは、Cognitoユーザープールを作成します。
今回はReactをベースにするので「シングルページアプリケーション（SPA）」を選びます。その他の値は任意ですが、今回はサインイン識別子をメールアドレスに設定しています。

![userpool0](/images/cognito-passkey/cognito-userpool_0.png)

## アプリケーションクライアントの設定

正常に作成されたら、一番下の「概要に移動」しユーザープールの設定を表示します。左のサイドバーからアプリケーションクライアントを選択し、認証フローに **選択ベースのサインイン** が有効化されていることを確認します

![userpool1](/images/cognito-passkey/cognito-userpool_1.png)

また、この後ローカルでの動作確認を行うため、**許可されているコールバックURL** および **許可されているサインアウト URL - オプション** に `http://localhost:5173/` を入力します

![userpool2](/images/cognito-passkey/cognito-userpool_2.png)

## パスキーの有効化

左のサイドバーから「認証」→「サインイン」を選び「選択ベースのサインインオプション」からパスキーを有効化します。

![userpool4](/images/cognito-passkey/cognito-userpool_4.png)

最後に以下の情報をメモしておきます。これらの情報は、フロントエンド側の設定に使用します

- ユーザープール ID
- クライアント ID
- Cognito ドメイン

# フロントエンド側のセットアップ

viteで簡単なテンプレートを作成します

```sh
npm create vite@latest my-passkey-app -- --template react
```

```sh
cd my-passkey-app
npm install
npm run dev
```

## Quick Setupに従う

CognitoのQuick Setupガイドに従ってフロントエンドを編集していきます。

![frontend0](/images/cognito-passkey/frontend_0.png)

**oidc-client-ts**と**react-oidc-context**のインストール

```sh
npm install oidc-client-ts react-oidc-context --save
```

AuthProviderとログインボタンの追加

```diff jsx:main.jsx
 import { StrictMode } from 'react'
 import { createRoot } from 'react-dom/client'
 import './index.css'
 import App from './App.jsx'
+ import { AuthProvider } from "react-oidc-context";

+ const cognitoAuthConfig = {
+   authority: "https://cognito-idp.ap-northeast-1.amazonaws.com/${ユーザープール ID}",
+   client_id: "${クライアント ID}",
+   redirect_uri: "http://localhost:5173/",
+   response_type: "code",
+   scope: "email openid phone",
+ };

 createRoot(document.getElementById('root')).render(
   <StrictMode>
+    <AuthProvider {...cognitoAuthConfig}>
       <App />
+    </AuthProvider>
   </StrictMode>,
 )
```

（元のapp.tsxの記述は邪魔なので全て消します）

```tsx:app.tsx
import './App.css'
import { useAuth } from "react-oidc-context";

function App() {
  const auth = useAuth();

  const signOutRedirect = () => {
    const clientId = "${クライアント ID}";
    const logoutUri = "http://localhost:5173/";
    const cognitoDomain = "${Cognito ドメイン}"
    window.location.href = `${cognitoDomain}/logout?client_id=${clientId}&logout_uri=${encodeURIComponent(logoutUri)}`;
  };

  if (auth.isLoading) {
    return <div>Loading...</div>;
  }

  if (auth.error) {
    return <div>Encountering error... {auth.error.message}</div>;
  }

  if (auth.isAuthenticated) {
    return (
      <div>
        <p> You're logged in. </p>
        <button onClick={() => auth.removeUser()}>Sign out</button>
      </div>
    );
  }

  return (
    <div>
      <button onClick={() => auth.signinRedirect()}>Sign in</button>
      <button onClick={() => signOutRedirect()}>Sign out</button>
    </div>
  );
}

export default App;
```

## Passkeyを登録する

http://localhost:5173/ にアクセスし、Sign inボタンからマネージドログイン画面が表示されることを確認します。
![frontend1](/images/cognito-passkey/frontend_1.png)

`Create an account`から適当なメールアドレスでアカウントを作成し、メール認証を済ませます。
アカウント作成に成功するとPasskeyの作成を促す画面が出てくるので、`Add passkey` からPasskeyを登録します。

![frontend2](/images/cognito-passkey/frontend_2.png)

二度目のサインイン時から、Passkeyかパスワード認証を選べるようになります。

![frontend3](/images/cognito-passkey/frontend_3.png)

## 後からPasskeyを登録する

この方法は、初回アカウント作成時しか利用できず、 `Set up sign-in with a passkey` で Not nowを選択してしてしまったり、マネコン側からユーザーを作成する場合は別途登録画面に遷移させる必要があります。

Passkey登録用ページに遷移させるためのボタンを設置し、後からでも登録できるようにします。

```diff tsx:app.tsx
 import './App.css'
 import { useAuth } from "react-oidc-context";
 
 function App() {
   const auth = useAuth();
 
   const signOutRedirect = () => {
     const clientId = "${クライアント ID}";
     const logoutUri = "http://localhost:5173/";
     const cognitoDomain = "${Cognito ドメイン}"
     window.location.href = `${cognitoDomain}/logout?client_id=${clientId}&logout_uri=${encodeURIComponent(logoutUri)}`;
   };

+  const setUpPasskey = () => {
+    const clientId = "${クライアント ID}";
+    const redirectUri = "http://localhost:5173/";
+    const cognitoDomain = "${Cognito ドメイン}"
+    window.location.href = `${cognitoDomain}/passkeys/add?client_id=${clientId}&redirect_uri=${encodeURIComponent(redirectUri)}`;
+  }
 
   if (auth.isLoading) {
     return <div>Loading...</div>;
   }
 
   if (auth.error) {
     return <div>Encountering error... {auth.error.message}</div>;
   }
 
   if (auth.isAuthenticated) {
     return (
       <div>
         <p> You're logged in. </p>
+        <button onClick={() => setUpPasskey()}>Set up Passkey</button>
         <button onClick={() => auth.removeUser()}>Sign out</button>
       </div>
     );
   }
 
   return (
     <div>
       <button onClick={() => auth.signinRedirect()}>Sign in</button>
       <button onClick={() => signOutRedirect()}>Sign out</button>
     </div>
   );
 }
 
 export default App;
```

https://docs.aws.amazon.com/cognito/latest/developerguide/amazon-cognito-user-pools-authentication-flow-methods.html#amazon-cognito-user-pools-authentication-flow-methods-passkey

パスキー登録用ボタンをクリックすると、先ほどと同じように登録ページに移動します。
![frontend4](/images/cognito-passkey/frontend_4.png)