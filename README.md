# Kubernetes構成可視化システムAPI(卒研引き継ぎ資料)
## 環境構築
このAPIでは，実際にk8sクラスターの構成情報を取得して整形して返却するため，
1. 取得元のk8sクラスターの環境構築
2. 本APIサーバーの環境構築

が必要になります．ひとつずつ手順を記します．
## k8sクラスターの環境構築
コントロールプレーンのためのマスターノードとデータプレーンのためのワーカーノードをそれぞれ用意します．卒研では以下の環境を構築しました．

(マスターノード，ワーカーノード共通, 仮想マシン)
- OS: ubuntu22.04
- メモリ: 4GB
- vCPU: 2
- ストレージ: 50GB

また，k8sを動かすためにインストールしたものは以下の通りです

(マスターノード，ワーカーノード共通)
- Kubernetes: Ver.1.26.1
- Containerd: Ver.1.6.15

k8s周りはアップデートが頻繁になされるので，これから説明する手順を踏んでも正しく動作しないことがありえるかと思います．適宜公式のドキュメントで調べるといいかと思います．
"[キーワード] doc"などで調べると公式documentにヒットするはずです．

以下実際に僕が入力したコマンドを記します．
### 【マスターノード，ワーカーノード共通】
ホスト名の登録
```
sudo vim /etc/hosts

以下を追加(IPアドレスは適宜読み替える，　ホスト名は各サーバーを作成した時に指定したホスト名を設定すると良いと思われる)
10.20.22.142 master
10.20.22.143 worker01
10.20.22.146 worker02
10.20.22.149 worker03
10.20.22.152 worker04
10.20.22.153 worker05
```

カーネルモジュールの有効化とswapの無効化
```
sudo modprobe overlay
sudo modprobe br_netfilter

cat <<EOF | sudo tee /etc/modules-load.d/k8s.conf
overlay
br_netfilter
EOF

cat <<EOF | sudo tee /etc/sysctl.d/k8s.conf
net.bridge.bridge-nf-call-iptables  = 1
net.bridge.bridge-nf-call-ip6tables = 1
net.ipv4.ip_forward                 = 1
EOF


sudo sed -i '/ swap / s/^\(.*\)$/#\1/g' /etc/fstab
sudo swapoff -a
free -m

---こうなってたらok---
               total        used        free      shared  buff/cache   available
Mem:            3925         255         826           1        2844        3378
Swap:              0           0           0
-------------------
```

Containerdのインストール
```
curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo gpg --dearmor -o /usr/share/keyrings/docker.gpg

echo \
  "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu \
  $(lsb_release -cs) stable" | sudo tee /etc/apt/sources.list.d/docker.list > /dev/null
  
sudo apt update

sudo apt install containerd.io

sudo systemctl stop containerd

sudo mv /etc/containerd/config.toml /etc/containerd/config.toml.orig
containerd config default | sudo tee /etc/containerd/config.toml

sudo vim /etc/containerd/config.toml
# 以下のように変更(/runc.optionsでエンターすると該当箇所が選択される)
[plugins."io.containerd.grpc.v1.cri".containerd.runtimes.runc]
  ...
  [plugins."io.containerd.grpc.v1.cri".containerd.runtimes.runc.options]
    SystemdCgroup = true
    
    
sudo systemctl start containerd

# 確認
sudo systemctl is-enabled containerd
## enabledだとok

sudo systemctl status containerd
## active(runnning)になってたらOK

```

k8sのインストール
```
sudo apt install apt-transport-https ca-certificates curl -y


sudo curl -fsSLo /usr/share/keyrings/kubernetes-archive-keyring.gpg https://packages.cloud.google.com/apt/doc/apt-key.gpg

echo "deb [signed-by=/usr/share/keyrings/kubernetes-archive-keyring.gpg] https://apt.kubernetes.io/ kubernetes-xenial main" | sudo tee /etc/apt/sources.list.d/kubernetes.list

sudo apt update

sudo apt install kubelet kubeadm kubectl
```

### 【ここからマスターノードだけ】
```
# カーネルモジュールの確認
lsmod | grep br_netfilter
---- これでOK----
br_netfilter           28672  0
bridge                299008  1 br_netfilter
----------------
sudo kubeadm config images pull

# クラスターの作成
# Podのネットワーク192.168.0.0/16でマスターノードのIPアドレスが10.20.22.142の時
sudo kubeadm init --pod-network-cidr=192.168.0.0/16 \
--apiserver-advertise-address=10.20.22.142 \
--cri-socket=unix:///run/containerd/containerd.sock

#上のコマンドの成功時に出力される"kubeadm join ~~(以下略)"のようなコマンドはワーカーノードをこのクラスターに追加する時に必要
# 説明上出力コマンドを（ア）とします．下で使います．

mkdir -p $HOME/.kube
sudo cp -i /etc/kubernetes/admin.conf $HOME/.kube/config
sudo chown $(id -u):$(id -g) $HOME/.kube/config
```

CNIプラグインのインストール(Calicoを使用)<br>CNIプラグインついては"k8sネットワーク実現方法"などで調べるとだんだんわかってくるかもしれないです．
```
kubectl create -f https://raw.githubusercontent.com/projectcalico/calico/v3.25.0/manifests/tigera-operator.yaml

curl https://raw.githubusercontent.com/projectcalico/calico/v3.25.0/manifests/custom-resources.yaml -O

kubectl create -f custom-resources.yaml
```

### 【ここからワーカーノードだけ】
```
# コマンド(ア)を入力
sudo kubeadm join 10.20.22.142:6443 --token 2t25a5.0xq43a28y1kgb7f1 --discovery-token-ca-cert-hash sha256:958209257be7f62464388399c68732d240dd957d992c294b85e4e4bda378d6ba

もし(ア)がわからなくなったらマスターノードで以下を入力すれば(ア)が取得可能
sudo kubeadm token create --print-join-command
```

### 【マスターノードで確認】
```
kubectl get nodes -o wide

# マスターノードとワーカーノードがリスト形式で表示されて，statusがreadyになっていたらOK
```

## 本APIサーバーの環境構築
卒研では開発はDockerを使用してローカル環境(個人PC)で行なって，完成したもののimageをビルドして，Docker HubにPushし，稼働用のサーバーにPullして，コンテナの作成，起動をさせました．
ローカル環境で開発した方が，vscodeなどのエディタを使って開発できるので，効率がいいと思います．また．Pushするイメージにはコンパイル後のバイナリのみをいれているので，余計なファイルがなく，かなり軽量で済むというメリットもあります．
### ローカル環境の構築
(特に機能の修正や追加を行わない場合はローカル環境の構築はスキップ．稼働用APIサーバーの構築へ)
1. 自身のPCにあった環境でDockerとdocker-composeを入れる
2. 対象のk8sクラスターを指定するために，このk8s-vis-backend配下に.kubeディレクトリを作成し，マスターノードの~/.kube/configをk8s-vis-backend/.kube配下にコピー
3. イメージのビルドをする 

```
# k8s-vis-backend配下で
docker-compose build
```
4. コンテナの作成起動
```
docker-compose up 
```

うまく作成できていたら，ブラウザで`localhost:8080/api/nodes`にアクセスするとでノード一覧取得ができます．(マスターノードと同一ネットワークである必要あり)

5. (機能実装)
6. イメージをビルドし，Docker HubにPushする
   (以下は私のアカウントですが，自身のDocker アカウントを作成してそこにPushした方がいいと思います．)
```
docker build -t api-prod --target prod .
docker tag api-prod asuyasuya/api-prod
docker push asuyasuya/api-prod
```
### 稼働用APIサーバーの構築
卒研では以下の環境で構築しました．(仮想マシン)
- OS: ubutnu22.04
- メモリ: 4GB
- vCPU: 2
- ストレージ: 50GB

1. Dockerを入れます.
[このサイト](https://qiita.com/yoshiyasu1111/items/17d9d928ceebb1f1d26d
)を参考にしました
2. イメージをPullする
```
docker pull asuyasuya/api-prod
```
3. Pullしたイメージから，コンテナ作成，起動
```
docker run --name api-prod-container -p 8080:8080 -d asuyasuya/api-prod
```

4. ローカルPCとAPIサーバーとk8sクラスターが同一ネットワークにある状態で，ローカルPCのブラウザで`10.20.22.192:8080/api/nodes`にアクセス(IPアドレスは自身の環境のAPIサーバーのIPアドレスを使用してください)

## 実験環境の構築(k8sクラスターの設定)
卒研における実験環境の構築手順を説明します．
シナリオとして下の3つがあります．
1. Nodeの数を変化させる
2. Podの数を変化させる
3. Network Policyの数を変化させる

teamsの`12_卒業生/2022年度卒業/B195312-懸川明日也/evaluation`配下にそれぞれのシナリオに対応する`node_change/`, `pod_change/`, `network_policy_change/`
があります．それらの中のyamlファイルをマスターノードに適用させたり，削除させたりすることで実験環境を変更させることができます．各シナリオのパラメータごとにさらにフォルダを作成しているので，それぞれのパラメータごとに適用するyamlファイルを変えてください.network_policy_changeディレクトリの各フォルダにはpod作成用のyamlファイル(nginx----.yaml)とpolicy作成用ののyamlファイル(policy----.yaml)があるので全て適用させてください．またnetwork_policy_changeのシナリオに限り， Pod詳細取得APIにおける対象Podを作成するためのnginx00.yamlの適用をしてください．

```
# 適用
kubectl apply -f example.yaml

# 削除
kubectl delete -f example.yaml
```

Podの数とNetwork Policyの数に関しては全てyamlファイルで変化させることができますが，Nodeの数に関してはマスターノード上でのコマンド実行で変更させる必要があります．Nodeの追加は上で述べた`kubeadm join`で可能であり，削除に関しては[このサイト](https://www.server-world.info/query?os=Ubuntu_20.04&p=kubernetes&f=8)
が参考になるかと思います．


### 計測時間について
計測の処理はすでにコードに含まれているので，ブラウザで`10.20.22.192:8080/api/nodes`のようにアクセスし実装したAPIを実行すれば処理時間が計測されその結果がログに出ます．ログの出力はAPIサーバーで以下のコマンドをコンテナが起動した状態で実行するとリアルタイムに確認することができます．
```
docker logs -f <起動しているコンテナ名>
```

詳しい計測タイミングは`src/controller/node_detail.go`, `src/controller/node_list.go`, `src/controller/pod_detail.go`を確認してもらえばわかると思います．time.Now()で計測を開始してtime.Since()までの経過時間を出力しています．
