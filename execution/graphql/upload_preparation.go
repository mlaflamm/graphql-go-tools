package graphql

import (
	"strings"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/astnormalization"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astnormalization/uploads"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvalidation"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/httpclient"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

func PrepareRequestForUploadExecution(request *Request, schema *Schema, files []*httpclient.FileUpload, validationOptions ...astvalidation.Option) error {
	if request == nil {
		return nil
	}

	if len(files) != 0 {
		request.files = files
	}

	if len(request.files) == 0 || request.uploadPreparationDone {
		return nil
	}

	result, err := request.Normalize(schema,
		astnormalization.WithRemoveFragmentDefinitions(),
		astnormalization.WithRemoveUnusedVariables(),
		astnormalization.WithInlineFragmentSpreads(),
	)
	if err != nil {
		return err
	}
	if !result.Successful {
		return result.Errors
	}

	validationResult, err := request.ValidateForSchema(schema, validationOptions...)
	if err != nil {
		return err
	}
	if !validationResult.Valid {
		return validationResult.Errors
	}

	uploadMapping, err := request.normalizeUploadVariables(schema)
	if err != nil {
		return err
	}

	updateUploadedFiles(request.files, uploadMapping)

	var remapReport operationreport.Report
	request.remapVariables = astnormalization.NewVariablesMapper().NormalizeOperation(request.Document(), schema.ClientDocument(), &remapReport)
	if remapReport.HasErrors() {
		return remapReport
	}

	remapUploadedFiles(request.files, request.remapVariables)
	request.uploadPreparationDone = true

	return nil
}

func updateUploadedFiles(files []*httpclient.FileUpload, uploadMapping []uploads.UploadPathMapping) {
	for i := range uploadMapping {
		if uploadMapping[i].NewUploadPath == "" {
			continue
		}

		for j := range files {
			if files[j].VariablePath() != uploadMapping[i].OriginalUploadPath {
				continue
			}

			files[j].SetVariablePath(uploadMapping[i].NewUploadPath)
			break
		}
	}
}

func remapUploadedFiles(files []*httpclient.FileUpload, remapVariables map[string]string) {
	if len(files) == 0 || len(remapVariables) == 0 {
		return
	}

	for i := range files {
		path := files[i].VariablePath()
		if !strings.HasPrefix(path, "variables.") {
			continue
		}

		remainder := strings.TrimPrefix(path, "variables.")
		variableName, nestedPath, hasNestedPath := strings.Cut(remainder, ".")

		for newName, oldName := range remapVariables {
			if oldName != variableName {
				continue
			}

			updatedPath := "variables." + newName
			if hasNestedPath {
				updatedPath += "." + nestedPath
			}

			files[i].SetVariablePath(updatedPath)
			break
		}
	}
}
