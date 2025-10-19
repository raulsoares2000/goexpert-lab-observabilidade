package tracer

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// InitTracerProvider inicializa e configura o provedor de traces do OpenTelemetry.
// Ele é responsável por criar os traces e exportá-los para um destino, como o OTEL Collector.
func InitTracerProvider(serviceName, collectorURL string) (*sdktrace.TracerProvider, error) {
	// Usamos context.Background() como o contexto pai, pois esta inicialização
	// deve viver durante todo o ciclo de vida da aplicação.
	ctx := context.Background()

	// resource.New cria um "recurso" que descreve a nossa aplicação.
	// Todos os spans gerados por este provider terão estes atributos.
	// O atributo mais importante é o `service.name`, que identifica o serviço no Zipkin.
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String(serviceName),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("falha ao criar recurso: %w", err)
	}

	// grpc.NewClient estabelece a conexão com o OTEL Collector no endereço fornecido.
	// Esta chamada é NÃO-BLOQUEANTE. A conexão será estabelecida em segundo plano.
	// A aplicação iniciará imediatamente, mesmo que o coletor não esteja pronto.
	// Isso torna a nossa aplicação mais resiliente.
	// Optamos por esta abordagem para seguir as melhores práticas do gRPC, que desaconselham
	// o uso da opção `grpc.WithBlock()`, pois pode bloquear o início da aplicação.
	conn, err := grpc.NewClient(collectorURL,
		// grpc.WithTransportCredentials(insecure.NewCredentials()) é usado para criar
		// uma conexão sem encriptação TLS. Adequado apenas para ambientes de desenvolvimento locais.
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("falha ao criar cliente gRPC para o coletor: %w", err)
	}

	// otlptracegrpc.New cria um exportador de traces que envia dados
	// usando o protocolo OTLP (OpenTelemetry Protocol) sobre a conexão gRPC que acabámos de configurar.
	traceExporter, err := otlptracegrpc.New(ctx, otlptracegrpc.WithGRPCConn(conn))
	if err != nil {
		return nil, fmt.Errorf("falha ao criar exportador de trace: %w", err)
	}

	// NewBatchSpanProcessor é um processador de spans que agrupa os spans em lotes (batches)
	// antes de os enviar para o exportador. Isto é muito mais eficiente do que enviar cada span individualmente.
	bsp := sdktrace.NewBatchSpanProcessor(traceExporter)

	// NewTracerProvider é o construtor principal do SDK. Ele junta a configuração do recurso,
	// o amostrador (sampler) e o processador de spans.
	tp := sdktrace.NewTracerProvider(
		// sdktrace.WithSampler(sdktrace.AlwaysSample()) configura o tracer para "amostrar",
		// ou seja, gravar e exportar 100% dos traces. Ótimo para ambientes de desenvolvimento e depuração.
		// Em produção, pode-se usar um amostrador baseado em probabilidade para reduzir o volume de dados.
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithResource(res),
		sdktrace.WithSpanProcessor(bsp),
	)

	// otel.SetTracerProvider define o TracerProvider que acabámos de criar como o provedor global
	// para toda a aplicação. Qualquer chamada a `otel.Tracer()` usará esta instância.
	otel.SetTracerProvider(tp)

	// otel.SetTextMapPropagator define o propagador global. O propagador é a peça mágica
	// que injeta e extrai o contexto de tracing (como Trace IDs e Span IDs) em cabeçalhos
	// de rede (ex: HTTP, gRPC). É isto que permite ligar os traces entre o Serviço A e o Serviço B.
	// TraceContext é o formato padrão e amplamente compatível.
	otel.SetTextMapPropagator(propagation.TraceContext{})

	// Retornamos o TracerProvider para que a função `main` que o chamou possa
	// gerir o seu ciclo de vida, especificamente chamando `Shutdown()` no final.
	return tp, nil
}
