# Sistema de Temperatura por CEP com Observabilidade

Sistema de microsserviços desenvolvido em Go para fornecer informações climáticas baseadas em CEP brasileiro, com observabilidade completa utilizando OpenTelemetry e Zipkin.

## 📋 Visão Geral

Este projeto demonstra uma arquitetura de microsserviços que consulta a temperatura atual de uma localidade a partir de um CEP brasileiro. Além da funcionalidade principal, implementa um sistema robusto de observabilidade com distributed tracing, permitindo monitoramento detalhado de performance e análise do fluxo de dados entre serviços.

## 🏗️ Arquitetura

A solução é composta por quatro componentes principais:

### Serviço A (Gateway)
- **Responsabilidade:** Porta de entrada da aplicação
- **Funcionalidades:** 
  - Recebe requisições contendo CEP
  - Valida formato do CEP (8 dígitos numéricos)
  - Encaminha requisições válidas para o Serviço B

### Serviço B (Orquestrador)
- **Responsabilidade:** Orquestração de chamadas a APIs externas
- **Funcionalidades:**
  - Consulta API ViaCEP para obter localidade
  - Consulta WeatherAPI para obter temperatura atual
  - Converte temperatura para Celsius, Fahrenheit e Kelvin
  - Retorna resposta formatada com todas as informações

### OTEL Collector
- **Responsabilidade:** Coleta e processamento de telemetria
- Recebe traces e spans dos serviços
- Exporta dados para backend de visualização

### Zipkin
- **Responsabilidade:** Visualização de distributed tracing
- Interface gráfica para análise de traces
- Formato waterfall para análise detalhada de requisições

## 🛠️ Tecnologias

- **Linguagem:** Go (Golang)
- **Roteamento HTTP:** Chi
- **Observabilidade:** OpenTelemetry (OTEL)
- **Visualização:** Zipkin
- **Containerização:** Docker & Docker Compose

## 🚀 Como Executar

### Pré-requisitos

- Docker e Docker Compose instalados
- Chave de API válida do [WeatherAPI](https://www.weatherapi.com/)

### Passos

1. Clone o repositório:
```bash
git clone <url-do-repositorio>
cd <nome-do-projeto>
```

2. Coloque sua chave API no arquivo `.env` na pasta service-b:
```env
WEATHER_API_KEY=SUA_CHAVE_DE_API_AQUI
```

3. Inicie os serviços:
```bash
docker-compose up --build
```

4. Aguarde a inicialização completa de todos os contêineres.

## 📡 Testando a Aplicação

### Endpoint Principal

```
POST http://localhost:8080/weather
```

### Request Body

```json
{
  "cep": "01001000"
}
```

### Cenários de Teste

#### ✅ Sucesso (CEP Válido)

**Request:**
```json
{
  "cep": "01001000"
}
```

**Response:** `200 OK`
```json
{
  "city": "São Paulo",
  "temp_C": 20.0,
  "temp_F": 68.0,
  "temp_K": 293.0
}
```

#### ❌ CEP Não Encontrado

**Request:**
```json
{
  "cep": "99999999"
}
```

**Response:** `404 Not Found`
```
can not find zipcode
```

#### ⚠️ CEP com Formato Inválido

**Request:**
```json
{
  "cep": "12345"
}
```

**Response:** `422 Unprocessable Entity`
```
invalid zipcode
```

## 🔍 Visualizando Observabilidade

1. Acesse a interface do Zipkin: **http://localhost:9411**

2. Clique em **"Run Query"** para buscar os traces disponíveis

3. Selecione um trace para visualizar:
   - Comunicação entre service-a e service-b
   - Chamadas para APIs externas (ViaCEP e WeatherAPI)
   - Métricas de tempo de cada span
   - Fluxo completo da requisição em formato cascata

## 📊 Estrutura de Traces

Cada requisição gera spans para:
- Recebimento da requisição no Serviço A
- Validação do CEP
- Chamada ao Serviço B
- Consulta à API ViaCEP
- Consulta à WeatherAPI
- Conversões de temperatura
- Retorno da resposta

## 📝 Notas

- Certifique-se de que a porta 8080 (Serviço A), 8081 (Serviço B), 4317 (OTEL Collector) e 9411 (Zipkin) estão disponíveis
- A chave da WeatherAPI deve ser válida e ativa
- Os logs de todos os serviços são exibidos no terminal durante a execução

