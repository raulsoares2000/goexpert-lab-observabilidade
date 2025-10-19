# Etapa 1: Build da aplicação
FROM golang:1.25-alpine as builder

# Define o diretório de trabalho dentro do contêiner
WORKDIR /app

# Copia os ficheiros de gestão de dependências
COPY go.mod go.sum ./

# Faz o download das dependências
RUN go mod download

# Copia o código fonte
COPY . .

# Compila a aplicação. O binário será estático e sem informações de debug.
RUN go build -ldflags="-w -s" -o /service-b ./service-b/main.go

# Etapa 2: Imagem final, otimizada
FROM alpine:latest

# Define o diretório de trabalho
WORKDIR /

# Copia apenas o binário compilado da etapa anterior
COPY --from=builder /service-b /service-b

# Expõe a porta do serviço B
EXPOSE 8081

# Comando para executar a aplicação
ENTRYPOINT ["/service-b"]