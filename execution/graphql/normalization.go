package graphql

import (
	"bytes"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/astnormalization"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astnormalization/uploads"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astprinter"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/graphqlerrors"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

type NormalizationResult struct {
	Successful bool
	Errors     graphqlerrors.Errors
}

func (r *Request) Normalize(schema *Schema, options ...astnormalization.Option) (result NormalizationResult, err error) {
	if schema == nil {
		return NormalizationResult{Successful: false, Errors: nil}, ErrNilSchema
	}

	report := r.parseQueryOnce()
	if report.HasErrors() {
		return NormalizationResultFromReport(report)
	}

	r.document.Input.Variables = r.Variables

	// use default normalization options if none are provided
	if len(options) == 0 {
		options = []astnormalization.Option{
			astnormalization.WithExtractVariables(),
			astnormalization.WithRemoveFragmentDefinitions(),
			astnormalization.WithRemoveUnusedVariables(),
			astnormalization.WithInlineFragmentSpreads(),
		}
	}

	if r.OperationName != "" {
		options = append(options, astnormalization.WithRemoveNotMatchingOperationDefinitions())
		normalizer := astnormalization.NewWithOpts(options...)
		normalizer.NormalizeNamedOperation(&r.document, schema.ClientDocument(), []byte(r.OperationName), &report)
	} else {
		// TODO: we should validate count of operations - to throw an error
		// and do full normalization for the single anonymous operation
		normalizer := astnormalization.NewWithOpts(options...)
		normalizer.NormalizeOperation(&r.document, schema.ClientDocument(), &report)
	}

	if report.HasErrors() {
		return NormalizationResultFromReport(report)
	}

	r.isNormalized = true

	r.Variables = r.document.Input.Variables

	return NormalizationResult{Successful: true, Errors: nil}, nil
}

func (r *Request) normalizeUploadVariables(schema *Schema) (uploadMapping []uploads.UploadPathMapping, err error) {
	if schema == nil {
		return nil, ErrNilSchema
	}

	report := r.parseQueryOnce()
	if report.HasErrors() {
		_, err = NormalizationResultFromReport(report)
		return nil, err
	}

	r.document.Input.Variables = r.Variables

	var operationBuf bytes.Buffer
	if err = astprinter.Print(&r.document, &operationBuf); err != nil {
		return nil, err
	}

	uploadDocument, uploadReport := astparser.ParseGraphqlDocumentBytes(operationBuf.Bytes())
	if uploadReport.HasErrors() {
		_, err = NormalizationResultFromReport(uploadReport)
		return nil, err
	}

	uploadDocument.Input.Variables = r.Variables
	uploadsMapping := astnormalization.NewVariablesNormalizer().NormalizeOperation(&uploadDocument, schema.ClientDocument(), &report)
	if report.HasErrors() {
		_, err = NormalizationResultFromReport(report)
		return nil, err
	}

	normalizer := astnormalization.NewWithOpts(astnormalization.WithExtractVariables())
	if r.OperationName != "" {
		normalizer.NormalizeNamedOperation(&r.document, schema.ClientDocument(), []byte(r.OperationName), &report)
	} else {
		normalizer.NormalizeOperation(&r.document, schema.ClientDocument(), &report)
	}
	if report.HasErrors() {
		_, err = NormalizationResultFromReport(report)
		return nil, err
	}

	r.Variables = r.document.Input.Variables

	return uploadsMapping, nil
}

func NormalizationResultFromReport(report operationreport.Report) (NormalizationResult, error) {
	result := NormalizationResult{
		Successful: false,
		Errors:     nil,
	}

	if !report.HasErrors() {
		result.Successful = true
		return result, nil
	}

	result.Errors = graphqlerrors.RequestErrorsFromOperationReport(report)

	var err error
	if len(report.InternalErrors) > 0 {
		err = report.InternalErrors[0]
	}

	return result, err
}
