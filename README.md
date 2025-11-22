# Kubernetes 기반 S3 파일 처리 AI Worker

이 프로젝트는 AWS S3에 파일이 업로드되면 이를 감지하여 쿠버네티스 환경에서 자동으로 처리하는 Go 언어 기반의 워커 애플리케이션입니다.

## 동작 방식

1.  **파일 업로드**: 사용자가 AWS S3 버킷에 파일을 업로드합니다.
2.  **이벤트 알림**: S3는 파일 생성 이벤트를 감지하여 연결된 AWS SQS(Simple Queue Service) 대기열(Queue)에 메시지를 보냅니다.
3.  **메시지 수신**: 쿠버네티스 클러스터에서 실행 중인 Go 워커가 SQS 대기열을 계속 감시하다가(Long Polling) 메시지를 수신합니다.
4.  **파일 다운로드**: 워커는 메시지에 포함된 S3 버킷 이름과 파일 경로 정보를 파싱하여 해당 파일을 컨테이너 내부의 임시 공간으로 다운로드합니다.
5.  **데이터 처리**: 다운로드된 파일을 기반으로 AI 추론, 데이터 변환 등 필요한 로직을 수행합니다. (현재 코드는 다운로드까지만 구현되어 있으며, 이 부분은 비즈니스 로직에 맞게 확장해야 합니다.)
6.  **메시지 삭제**: 모든 처리가 완료되면, 동일한 작업이 반복되지 않도록 SQS 대기열에서 해당 메시지를 삭제합니다.

## 주요 기능

-   Go 언어를 사용한 경량 컨테이너 구현
-   AWS SQS 메시지 수신 및 처리 (Long Polling)
-   AWS S3 이벤트 정보 파싱 및 파일 다운로드
-   `Dockerfile`을 통한 손쉬운 컨테이너 이미지 빌드
-   `Kubernetes` 배포를 위한 YAML 매니페스트 제공

---

## 사용 방법

### 사전 준비 사항

-   **AWS**
    -   파일을 업로드할 S3 버킷
    -   S3 파일 생성 이벤트를 수신할 SQS 대기열
    -   위 S3와 SQS에 접근 가능한 AWS IAM 자격 증명 (Access Key ID, Secret Access Key)
-   **로컬 환경**
    -   Docker
    -   Kubernetes 클러스터 
    -   kubectlCLI

### 1. AWS 설정: S3 -> SQS 이벤트 연동

-   SQS 대기열을 먼저 생성합니다.
-   S3 버킷의 **속성 > 이벤트 알림 > 이벤트 알림 생성** 메뉴로 이동하여, 모든 객체 생성 이벤트(`s3:ObjectCreated:*`)가 위에서 생성한 SQS 대기열로 전송되도록 설정합니다.

### 2. 쿠버네티스 Secret 생성

워커가 AWS에 접근하려면 자격 증명이 필요합니다. 아래 명령어를 실행하여 대신 실행해줄 bot을 쿠버네티스 Secret을 생성합니다.


```bash
kubectl create secret generic aws-sqs-secret \
  --from-literal=AWS_ACCESS_KEY_ID='여기에_ACCESS_KEY_ID_입력' \
  --from-literal=AWS_SECRET_ACCESS_KEY='여기에_SECRET_ACCESS_KEY_입력' \
  --from-literal=AWS_REGION='ap-northeast-2' # 또는 사용하는 리전
```
> `deploy.yaml` 파일이 이 `aws-sqs-secret`을 참조하여 컨테이너에 환경변수로 주입합니다.

### 3. Docker 이미지 빌드 및 푸시

아래 명령어로 Go 애플리케이션의 Docker 이미지를 빌드하고, Docker Hub 또는 사용 중인 컨테이너 레지스트리에 푸시합니다.

```bash
# Docker 이미지 빌드
docker build --platform linux/amd64 -t 본인ID/sqs-worker:v1 .

# Docker Hub에 로그인
docker login

# 이미지 푸시
docker push YOUR_DOCKER_ID/sqs-worker:v1
```
> `YOUR_DOCKER_ID` 부분을 실제 Docker Hub ID 또는 레지스트리 주소로 변경해주세요.

### 4. Kubernetes 배포 파일 수정

`Kubernetes_yaml/deploy.yaml` 파일을 열어 아래 두 부분을 수정합니다.

1.  `spec.template.spec.containers.image`: 방금 푸시한 **Docker 이미지 주소**로 변경합니다.
    ```yaml
    containers:
    - name: ai-worker
      image: YOUR_DOCKER_ID/sqs-worker:v1
    ```

2.  `spec.template.spec.containers.env.value`: **SQS 대기열의 URL**을 입력합니다.
    ```yaml
    env:
    - name: SQS_QUEUE_URL
      value: "https://sqs.ap-northeast-2.amazonaws.com/..." 
    ```
    
### 5. 쿠버네티스에 배포

아래 명령어로 쿠버네티스 클러스터에 워커를 배포합니다.

```bash
kubectl apply -f Kubernetes_yaml/deploy.yaml
```

### 6. 실행 확인

파드가 정상적으로 실행되고 있는지 확인하고, 로그를 통해 메시지 수신 상태를 모니터링합니다.

```bash
# 파드 상태 확인
kubectl get pods -l app=ai-worker

# 파드 이름 확인 후 로그 출력 (실시간)
kubectl logs -f <파드_이름>
```

S3 버킷에 파일을 업로드하면, 잠시 후 터미널에 메시지 수신 및 파일 다운로드 로그가 출력되는 것을 확인할 수 있습니다.

## 프로젝트 구조

```
.
├── Dockerfile              # 컨테이너 이미지 빌드를 위한 Dockerfile
├── go.mod                  # Go 모듈 의존성 관리 파일
├── go.sum
├── Kubernetes_yaml/
│   └── deploy.yaml         # 쿠버네티스 배포를 위한 매니페스트
├── main.go                 # SQS 메시지 처리 로직을 담은 메인 애플리케이션
└── README.md               # 프로젝트 설명 파일
```

<img width="890" height="111" alt="스크린샷 2025-11-22 오후 3 27 25" src="https://github.com/user-attachments/assets/22017768-a298-40e8-ac56-8f25a7c997fd" />
