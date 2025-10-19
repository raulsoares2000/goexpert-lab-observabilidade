package main

import (
	"Observabilidade/tracer"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// CEPRequest define a estrutura do JSON que esperamos receber no corpo da requisição.
type CEPRequest struct {
	CEP string `json:"cep"`
}

func main() {
	// --- Início da Configuração do OpenTelemetry ---
	// Lemos o endereço do OTEL Collector a partir das variáveis de ambiente,
	// que serão injetadas pelo docker-compose.yml.
	collectorURL := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if collectorURL == "" {
		collectorURL = "localhost:4317" // Fallback para execuções locais fora do Docker.
	}

	// Inicializamos o Tracer Provider para o "service-a".
	// A função `InitTracerProvider` vem do nosso pacote partilhado `tracer`.
	tp, err := tracer.InitTracerProvider("service-a", collectorURL)
	if err != nil {
		log.Fatalf("falha ao inicializar tracer provider: %v", err)
	}
	// `defer` garante que o `Shutdown` será chamado quando a função `main` terminar,
	// assegurando que todos os spans em buffer sejam enviados.
	defer func() {
		if err := tp.Shutdown(context.Background()); err != nil {
			log.Printf("erro ao desligar tracer provider: %v", err)
		}
	}()
	// --- Fim da Configuração do OpenTelemetry ---

	// Configuramos o router HTTP usando a biblioteca Chi.
	r := chi.NewRouter()
	r.Use(middleware.Logger) // Adiciona um logger para cada requisição.

	// Criamos um handler que envolve a nossa lógica (`GetWeatherViaServiceB`) com o middleware do OTEL.
	// Este middleware cria automaticamente um span para cada requisição recebida por este serviço.
	// O nome "WeatherHandler" será o nome do span principal no Zipkin para este serviço.
	otelHandler := otelhttp.NewHandler(http.HandlerFunc(GetWeatherViaServiceB), "WeatherHandler")

	// Mapeamos a rota POST /weather para o nosso handler instrumentado.
	r.Post("/weather", otelHandler.ServeHTTP)

	fmt.Println("Serviço A está a correr na porta 8080...")
	http.ListenAndServe(":8080", r)
}

// GetWeatherViaServiceB é o handler que processa a requisição.
func GetWeatherViaServiceB(w http.ResponseWriter, r *http.Request) {
	// O contexto `r.Context()` já contém as informações do span criado pelo middleware otelHandler.
	ctx := r.Context()

	var req CEPRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Validamos o formato do CEP.
	if !isValidCEP(req.CEP) {
		http.Error(w, "invalid zipcode", http.StatusUnprocessableEntity) // [cite: 4]
		return
	}

	// Criamos um cliente HTTP cujo transporte é instrumentado pelo OTEL.
	// `otelhttp.NewTransport` envolve o transporte HTTP padrão. Ele automaticamente
	// injeta os cabeçalhos de propagação de contexto (Trace ID, Span ID) na requisição
	// que será feita para o Serviço B. É isto que conecta os dois traces.
	client := http.Client{Transport: otelhttp.NewTransport(http.DefaultTransport)}

	// Montamos a URL para chamar o Serviço B. "service-b" é o nome do container no docker-compose.
	url := fmt.Sprintf("http://service-b:8081/weather/%s", req.CEP)
	httpReq, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		http.Error(w, "erro ao criar requisição para o serviço B", http.StatusInternalServerError)
		return
	}

	// Executamos a chamada. O span gerado por esta chamada será filho do span "WeatherHandler".
	resp, err := client.Do(httpReq)
	if err != nil {
		http.Error(w, "erro ao chamar o serviço B", http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	// Simplesmente repassamos a resposta (cabeçalhos, status e corpo) do Serviço B
	// de volta para o cliente original.
	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

// isValidCEP valida se a string do CEP contém exatamente 8 dígitos numéricos.
func isValidCEP(cep string) bool {
	match, _ := regexp.MatchString("^[0-9]{8}$", cep)
	return match
}
