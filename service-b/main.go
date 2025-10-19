package main

import (
	trc "Observabilidade/tracer"
	"context"
	"encoding/json"
	"fmt"
	net_url "net/url"
	"regexp"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// ViaCEPResponse é uma struct para receber a resposta da API ViaCEP
type ViaCEPResponse struct {
	Localidade string `json:"localidade"`
	Erro       string `json:"erro"`
}

// WeatherAPIResponse é uma struct para receber a resposta da API WeatherAPI
type WeatherAPIResponse struct {
	Current struct {
		TempC float64 `json:"temp_c"`
	} `json:"current"`
}

// FinalResponse é uma struct para a nossa resposta final
type FinalResponse struct {
	City  string  `json:"city"`
	TempC float64 `json:"temp_C"`
	TempF float64 `json:"temp_F"`
	TempK float64 `json:"temp_K"`
}

func main() {
	// Acessa a chave da API a partir de uma variável de ambiente
	apiKey := os.Getenv("WEATHER_API_KEY")
	if apiKey == "" {
		log.Fatal("API key not configured")
		return
	}

	// Configuração do OpenTelemetry, idêntica à do Serviço A,
	// mas identificando-se como "service-b".
	collectorURL := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if collectorURL == "" {
		collectorURL = "localhost:4317"
	}
	tp, err := trc.InitTracerProvider("service-b", collectorURL)
	if err != nil {
		log.Fatalf("falha ao inicializar tracer provider: %v", err)
	}
	defer func() {
		if err := tp.Shutdown(context.Background()); err != nil {
			log.Printf("erro ao desligar tracer provider: %v", err)
		}
	}()

	// Cria um router usando o Chi
	r := chi.NewRouter()
	r.Use(middleware.Logger) // Middleware para logar as requisições

	// Define a rota e o handler correspondente
	r.Get("/weather/{cep}", GetWeatherHandler)

	// O middleware do OTEL irá extrair o contexto de trace dos cabeçalhos da requisição
	// vinda do Serviço A e criar um span filho, continuando o trace distribuído.
	otelHandler := otelhttp.NewHandler(http.HandlerFunc(GetWeatherHandler), "WeatherHandler")
	r.Handle("/weather/{cep}", otelHandler)

	fmt.Println("Serviço B está a correr na porta 8081...")
	err = http.ListenAndServe(":8081", r)
	if err != nil {
		fmt.Println("Erro ao iniciar o servidor:", err)
		return
	}
}

// GetWeatherHandler é o handler principal que orquestra as chamadas
func GetWeatherHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	// Obtemos uma instância do tracer para criar spans personalizados.
	tracer := otel.Tracer("service-b-tracer")

	// Obtém o CEP do parâmetro da URL
	cep := chi.URLParam(r, "cep")
	if !isValidCEP(cep) {
		http.Error(w, "invalid zipcode", http.StatusUnprocessableEntity)
		return
	}

	// Obtemos o span atual a partir do contexto para adicionar atributos a ele.
	span := trace.SpanFromContext(ctx)
	span.SetAttributes(attribute.String("cep", cep))

	// Busca a localização (cidade) usando o ViaCEP
	location, err := fetchLocation(ctx, tracer, cep)
	if err != nil {
		if err.Error() == "can not find zipcode" {
			http.Error(w, "can not find zipcode", http.StatusNotFound)
		} else {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	// Busca a temperatura usando a WeatherAPI
	weather, err := fetchWeather(ctx, tracer, location.Localidade)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Calcula as temperaturas em Fahrenheit e Kelvin
	tempC := weather.Current.TempC
	tempF := tempC*1.8 + 32
	tempK := tempC + 273

	// Monta a resposta final
	response := FinalResponse{
		City:  location.Localidade,
		TempC: tempC,
		TempF: tempF,
		TempK: tempK,
	}

	// Define o cabeçalho como JSON e envia a resposta
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	err = json.NewEncoder(w).Encode(response)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

// fetchLocation busca a cidade com base no CEP
func fetchLocation(ctx context.Context, tr trace.Tracer, cep string) (*ViaCEPResponse, error) {
	// Criamos um novo span filho chamado "fetchLocation-viacep".
	// Este span aparecerá aninhado dentro do span "WeatherHandler" do Serviço B no Zipkin.
	ctx, span := tr.Start(ctx, "fetchLocation-viacep")
	defer span.End() // Garante que o span seja finalizado ao sair da função.

	// Monta a URL da API ViaCEP
	url := fmt.Sprintf("https://viacep.com.br/ws/%s/json/", cep)

	// Usamos `http.NewRequestWithContext` para garantir que o contexto do nosso trace
	// (e qualquer prazo ou cancelamento) seja propagado para a chamada HTTP.
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	// Executamos a requisição usando o cliente HTTP padrão.
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		// Se houver um erro de rede ou na chamada, retornamos.
		return nil, err
	}
	// `defer resp.Body.Close()` é uma prática padrão para garantir que a conexão seja fechada.
	defer resp.Body.Close()

	// Lemos todo o corpo da resposta.
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Converte o JSON para a struct
	var viaCEPResponse ViaCEPResponse
	if err = json.Unmarshal(body, &viaCEPResponse); err != nil {
		return nil, err
	}

	// Verifica se o ViaCEP retornou um erro (CEP não encontrado)
	if viaCEPResponse.Erro == "true" {
		return nil, fmt.Errorf("can not find zipcode")
	}

	return &viaCEPResponse, nil
}

// fetchWeather busca a temperatura com base na cidade
func fetchWeather(ctx context.Context, tr trace.Tracer, city string) (*WeatherAPIResponse, error) {
	// Criamos outro span filho, desta vez para a chamada à WeatherAPI.
	// No Zipkin, ele aparecerá no mesmo nível que o span `fetchLocation-viacep`.
	ctx, span := tr.Start(ctx, "fetchWeather-weatherapi")
	defer span.End()

	// Obtém a chave da API das variáveis de ambiente
	apiKey := os.Getenv("WEATHER_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("WEATHER_API_KEY não definida")
	}

	// A função url.QueryEscape garante que caracteres especiais na cidade (como espaços ou acentos)
	// sejam codificados corretamente para a URL. Ex: "São Paulo" -> "S%C3%A3o%20Paulo"
	encodedCity := net_url.QueryEscape(city)

	// Monta a URL da API WeatherAPI
	url := fmt.Sprintf("http://api.weatherapi.com/v1/current.json?key=%s&q=%s&aqi=no", apiKey, encodedCity)

	// Novamente, usamos `http.NewRequestWithContext` para propagar o trace.
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Lê o corpo da resposta
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("erro ao ler resposta da WeatherAPI: %w", err)
	}

	// Converte o JSON para a struct
	var weatherAPIResponse WeatherAPIResponse
	if err = json.Unmarshal(body, &weatherAPIResponse); err != nil {
		return nil, fmt.Errorf("erro ao decodificar JSON da WeatherAPI: %w", err)
	}

	return &weatherAPIResponse, nil
}

func isValidCEP(cep string) bool {
	// A expressão regular ^[0-9]{8}$ verifica o formato completo.
	match, _ := regexp.MatchString("^[0-9]{8}$", cep)
	return match
}
