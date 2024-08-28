package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
)

// Define the base URL as a constant
const baseURL = "https://api.mfapi.in/mf/"

// Define a Fund struct to match the API response structure
type Fund struct {
	Meta struct {
		FundHouse      string `json:"fund_house"`
		SchemeType     string `json:"scheme_type"`
		SchemeCategory string `json:"scheme_category"`
		SchemeCode     int    `json:"scheme_code"`
		SchemeName     string `json:"scheme_name"`
	} `json:"meta"`
	Data []struct {
		Date string `json:"date"`
		Nav  string `json:"nav"`
	} `json:"data"`
}

// Define a Response struct for the API response
type Response struct {
	Meta struct {
		FundHouse      string `json:"fund_house"`
		SchemeType     string `json:"scheme_type"`
		SchemeCategory string `json:"scheme_category"`
		SchemeCode     int    `json:"scheme_code"`
		SchemeName     string `json:"scheme_name"`
	} `json:"meta"`
	Period string      `json:"period,omitempty"`
	Data   []DataPoint `json:"data"`
}

// Define a DataPoint struct for individual data points
type DataPoint struct {
	Date string `json:"date"`
	Nav  string `json:"nav"`
}

// Handler function to process the API Gateway request
func Handler(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	// Fetch query parameters
	mutualFundID := request.QueryStringParameters["mutualFundID"]
	startDate := request.QueryStringParameters["start"]
	endDate := request.QueryStringParameters["end"]

	// Check if mutualFundID is provided and is a valid integer
	if mutualFundID == "" {
		return createErrorResponse(400, "mutualFundID query parameter is required")
	}
	if !isValidInteger(mutualFundID) {
		return createErrorResponse(400, "mutualFundID must be an integer")
	}

	// Validate and parse dates
	start, end, err := validateAndParseDates(startDate, endDate)
	if err != nil {
		return createErrorResponse(400, err.Error())
	}

	// Fetch fund data from API
	fund, err := fetchFundData(mutualFundID)
	if err != nil {
		return createErrorResponse(500, err.Error())
	}

	// Check if the meta field is empty, indicating an invalid mutualFundID
	if fund.Meta == (struct {
		FundHouse      string `json:"fund_house"`
		SchemeType     string `json:"scheme_type"`
		SchemeCategory string `json:"scheme_category"`
		SchemeCode     int    `json:"scheme_code"`
		SchemeName     string `json:"scheme_name"`
	}{}) {
		return createErrorResponse(400, "Invalid mutualFundID")
	}

	// Filter data based on date range
	filteredData := filterData(fund.Data, start, end)

	// Create a success response
	response := createSuccessResponse(fund.Meta, filteredData, start, end)

	// Marshal the response to JSON
	jsonResponse, err := json.Marshal(response)
	if err != nil {
		return createErrorResponse(500, "error creating JSON response")
	}

	return events.APIGatewayProxyResponse{
		StatusCode: 200,
		Body:       string(jsonResponse),
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
	}, nil
}

// isValidInteger checks if a string can be parsed as an integer.
func isValidInteger(value string) bool {
	_, err := strconv.Atoi(value)
	return err == nil
}

// fetchFundData fetches the fund data from the API using the mutualFundID.
func fetchFundData(mutualFundID string) (Fund, error) {
	var fund Fund

	url := fmt.Sprintf("%s%s", baseURL, mutualFundID)
	resp, err := http.Get(url)
	if err != nil {
		return fund, fmt.Errorf("error fetching data from API: %w", err)
	}
	defer resp.Body.Close()

	if err := json.NewDecoder(resp.Body).Decode(&fund); err != nil {
		return fund, fmt.Errorf("error decoding API response: %w", err)
	}

	return fund, nil
}

// validateAndParseDates validates and parses the date strings from the query parameters.
func validateAndParseDates(startDate, endDate string) (time.Time, time.Time, error) {
	var start, end time.Time
	var err error

	if startDate != "" && endDate != "" {
		start, end, err = parseDates(startDate, endDate)
		if err != nil {
			return time.Time{}, time.Time{}, err
		}

		if end.After(time.Now()) {
			return time.Time{}, time.Time{}, fmt.Errorf("end date cannot be in the future")
		}

		if start.After(end) {
			return time.Time{}, time.Time{}, fmt.Errorf("start date cannot be after end date")
		}
	} else if startDate == "" && endDate == "" {
		// No dates provided, return all data
		start, end = time.Time{}, time.Time{}
	} else {
		// Only one of the dates provided
		return time.Time{}, time.Time{}, fmt.Errorf("both start and end dates are required in the format dd-mm-yyyy")
	}

	return start, end, nil
}

// parseDates parses the start and end date strings into time.Time objects.
func parseDates(startDate, endDate string) (time.Time, time.Time, error) {
	start, err := time.Parse("02-01-2006", startDate)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid start date format. use dd-mm-yyyy")
	}

	end, err := time.Parse("02-01-2006", endDate)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid end date format. use dd-mm-yyyy")
	}

	return start, end, nil
}

// filterData filters the data based on the provided date range.
func filterData(data []struct {
	Date string `json:"date"`
	Nav  string `json:"nav"`
}, start, end time.Time) []DataPoint {
	var filteredData []DataPoint

	for _, item := range data {
		date, err := time.Parse("02-01-2006", item.Date)
		if err != nil {
			continue
		}
		if (start.IsZero() && end.IsZero()) || (date.Equal(start) || date.After(start)) && (date.Equal(end) || date.Before(end)) {
			filteredData = append(filteredData, DataPoint{Date: item.Date, Nav: item.Nav})
		}
	}

	return filteredData
}

// createSuccessResponse creates a successful response with the given data and period.
func createSuccessResponse(meta struct {
	FundHouse      string `json:"fund_house"`
	SchemeType     string `json:"scheme_type"`
	SchemeCategory string `json:"scheme_category"`
	SchemeCode     int    `json:"scheme_code"`
	SchemeName     string `json:"scheme_name"`
}, data []DataPoint, start, end time.Time) Response {
	response := Response{
		Meta: meta,
		Data: data,
	}

	if !start.IsZero() && !end.IsZero() {
		response.Period = fmt.Sprintf("%s to %s", start.Format("02-01-2006"), end.Format("02-01-2006"))
	}

	return response
}

// createErrorResponse creates an error response with the given status code and message.
func createErrorResponse(statusCode int, message string) (events.APIGatewayProxyResponse, error) {
	return events.APIGatewayProxyResponse{
		StatusCode: statusCode,
		Body:       fmt.Sprintf(`{"error": "%s"}`, message),
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
	}, nil
}

func main() {
	lambda.Start(Handler)
}
