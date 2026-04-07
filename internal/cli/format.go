package cli

import "flag"

type outputFormat string

const (
	formatJSON outputFormat = "json"
	formatText outputFormat = "text"
)

func addFormatFlag(flags *flag.FlagSet) *string {
	return flags.String("format", string(formatJSON), "Output format: json or text.")
}

func validateFormat(value string) errorResponse {
	response := errorResponse{Status: "error"}
	response.Error.Code = "invalid_usage"
	response.Error.Message = "unsupported format " + `"` + value + `"`
	response.Error.Details = map[string]any{
		"supported_formats": []string{string(formatJSON), string(formatText)},
	}
	return response
}
