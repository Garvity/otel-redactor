// Package collector assembles the runnable Collector distribution.
package collector

import (
	"github.com/Garvity/otel-redactor/processor/otelredactorprocessor"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/confmap/provider/envprovider"
	"go.opentelemetry.io/collector/confmap/provider/fileprovider"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/exporter/debugexporter"
	"go.opentelemetry.io/collector/exporter/otlphttpexporter"
	"go.opentelemetry.io/collector/otelcol"
	"go.opentelemetry.io/collector/processor"
	"go.opentelemetry.io/collector/processor/batchprocessor"
	"go.opentelemetry.io/collector/processor/memorylimiterprocessor"
	"go.opentelemetry.io/collector/receiver"
	"go.opentelemetry.io/collector/receiver/otlpreceiver"
	"go.opentelemetry.io/collector/service/telemetry/otelconftelemetry"
)

// Factories exposes only the components shipped in this distribution.
func Factories() (otelcol.Factories, error) {
	var f otelcol.Factories
	var err error
	f.Receivers, err = otelcol.MakeFactoryMap[receiver.Factory](otlpreceiver.NewFactory())
	if err != nil {
		return f, err
	}
	f.Processors, err = otelcol.MakeFactoryMap[processor.Factory](memorylimiterprocessor.NewFactory(), otelredactorprocessor.NewFactory(), batchprocessor.NewFactory())
	if err != nil {
		return f, err
	}
	f.Exporters, err = otelcol.MakeFactoryMap[exporter.Factory](debugexporter.NewFactory(), otlphttpexporter.NewFactory())
	if err != nil {
		return f, err
	}
	f.Telemetry = otelconftelemetry.NewFactory()
	return f, nil
}

// Settings is shared by the CLI and the real-network demo/integration tests.
func Settings(version string, configURIs ...string) otelcol.CollectorSettings {
	return otelcol.CollectorSettings{
		Factories: Factories,
		BuildInfo: component.BuildInfo{Command: "otel-redactor", Description: "Recursive trace redaction Collector", Version: version},
		ConfigProviderSettings: otelcol.ConfigProviderSettings{
			ResolverSettings: confmap.ResolverSettings{
				URIs:              configURIs,
				ProviderFactories: []confmap.ProviderFactory{fileprovider.NewFactory(), envprovider.NewFactory()},
				DefaultScheme:     "env",
			},
		},
	}
}
