package function

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"cloud.google.com/go/compute/metadata"

	"google.golang.org/api/run/v1"

	"github.com/GoogleCloudPlatform/functions-framework-go/functions"
	"github.com/cloudevents/sdk-go/v2/event"

	"github.com/slack-go/slack"
)

// // AuditLogEntry represents a LogEntry as described at
// // https://cloud.google.com/logging/docs/reference/v2/rest/v2/LogEntry
// type AuditLogEntry struct {
// 	ProtoPayload *AuditLogProtoPayload `json:"protoPayload"`
// }

// // AuditLogProtoPayload represents AuditLog within the LogEntry.protoPayload
// // See https://cloud.google.com/logging/docs/reference/audit/auditlog/rest/Shared.Types/AuditLog
// type AuditLogProtoPayload struct {
// 	MethodName         string                 `json:"methodName"`
// 	ResourceName       string                 `json:"resourceName"`
// 	AuthenticationInfo map[string]interface{} `json:"authenticationInfo"`
// }

type AuditLogEntry struct {
	ProtoPayload struct {
		MethodName         string `json:"methodName"`
		ResourceName       string `json:"resourceName"`
		ServiceName        string `json:"serviceName"`
		AuthenticationInfo struct {
			PrincipalEmail string `json:"principalEmail"`
		} `json:"authenticationInfo"`
		Request struct {
			Service struct {
				APIVersion string `json:"apiVersion"`
				Kind       string `json:"kind"`
				Metadata   struct {
					Name        string            `json:"name"`
					Namespace   string            `json:"namespace"`
					Annotations map[string]string `json:"annotations"`
				} `json:"metadata"`
				Spec struct {
					Template struct {
						Metadata struct {
							Name        string            `json:"name"`
							Annotations map[string]string `json:"annotations"`
						} `json:"metadata"`
						Spec struct {
							ContainerConcurrency int    `json:"containerConcurrency"`
							TimeoutSeconds       int    `json:"timeoutSeconds"`
							ServiceAccountName   string `json:"serviceAccountName"`
						} `json:"spec"`
					} `json:"template"`
				} `json:"spec"`
			} `json:"service"`
			Region string `json:"region"`
			Type   string `json:"@type"`
			Parent string `json:"parent"`
		} `json:"request"`
		Response struct {
			Metadata struct {
				Name              string    `json:"name"`
				Namespace         string    `json:"namespace"`
				SelfLink          string    `json:"selfLink"`
				UID               string    `json:"uid"`
				ResourceVersion   string    `json:"resourceVersion"`
				Generation        int       `json:"generation"`
				CreationTimestamp time.Time `json:"creationTimestamp"`
				Labels            struct {
					CloudGoogleapisComLocation string `json:"cloud.googleapis.com/location"`
				} `json:"labels"`
				Annotations map[string]string `json:"annotations"`
				// } `json:"annotations"`
			} `json:"metadata"`
			APIVersion string `json:"apiVersion"`
			Type       string `json:"@type"`
			Kind       string `json:"kind"`
			Spec       struct {
				Template struct {
					Metadata struct {
						Name string `json:"name"`
						Annotations map[string]string `json:"annotations"`
					} `json:"metadata"`
					Spec struct {
						ContainerConcurrency int    `json:"containerConcurrency"`
						TimeoutSeconds       int    `json:"timeoutSeconds"`
						ServiceAccountName   string `json:"serviceAccountName"`
					} `json:"spec"`
				} `json:"template"`
				Traffic []struct {
					Percent        int  `json:"percent"`
					LatestRevision bool `json:"latestRevision"`
				} `json:"traffic"`
			} `json:"spec"`
			Status struct {
			} `json:"status"`
		} `json:"response"`
		ResourceLocation struct {
			CurrentLocations []string `json:"currentLocations,omitempty"`
		} `json:"resourceLocation,omitempty"`
	} `json:"protoPayload,omitempty"`
	Resource struct {
		Type   string `json:"type,omitempty"`
		Labels struct {
			ConfigurationName string `json:"configuration_name,omitempty"`
			ProjectID         string `json:"project_id,omitempty"`
			ServiceName       string `json:"service_name,omitempty"`
			Location          string `json:"location,omitempty"`
			RevisionName      string `json:"revision_name,omitempty"`
		} `json:"labels,omitempty"`
	} `json:"resource,omitempty"`
}

func sendToSlackChannel(payload *string) (err error) {
	channelId := os.Getenv("SLACK_CHANNEL_ID")
	if channelId == "" {
		return fmt.Errorf("SLACK_CHANNEL_ID is null")
	}
	oauthToken := os.Getenv("SLACK_BOT_USER_OAUTH_TOKEN")
	if oauthToken == "" {
		return fmt.Errorf("SLACK_BOT_USER_OAUTH_TOKEN is null")
	}
	api := slack.New(oauthToken)
	// https://api.slack.com/reference/surfaces/formatting#escaping
	_, _, err = api.PostMessage(
		channelId,
		slack.MsgOptionText(*payload, false),
	)
	if err != nil {
		return fmt.Errorf("%s %v", "slack.api.PostMessage", err)
	}
	return nil
}

func init() {
	functions.CloudEvent("CloudEventFunc", cloudEventFunc)
}

func cloudEventFunc(ctx context.Context, e event.Event) error {

	var err error
	var projectId string

	projectId = os.Getenv("PROJECT_ID")
	if projectId == "" {
		fmt.Printf("PROJECT_ID is empty\n")

		c := metadata.NewClient(&http.Client{})
		projectId, err = c.ProjectID()
		if err != nil {
			return fmt.Errorf("failed get metadata.ProjectID: %v", err)
		}
	}

	fmt.Printf("PROJECT_ID: %s\n", projectId)

	log.Printf("Event ID: %s", e.ID())
	log.Printf("Event Type: %s", e.Type())

	logentry := &AuditLogEntry{}
	if err := e.DataAs(&logentry); err != nil {
		return fmt.Errorf("event.DataAs: %v", err)
	}

	resource := &logentry.Resource
	payload := &logentry.ProtoPayload

	log.Print("PrincipalEmail: ", payload.AuthenticationInfo.PrincipalEmail)
	log.Print("MethodName: ", payload.MethodName)
	log.Print("ResourceName: ", payload.ResourceName)
	log.Print("Request.Region: ", payload.Request.Region)

	runService, err := run.NewService(context.Background())
	if err != nil {
		return fmt.Errorf("failed to create services client: %v", err)
	}

	project := resource.Labels.ProjectID
	location := resource.Labels.Location
	serviceName := resource.Labels.ServiceName

	log.Printf("Project: %s | Location: %s | Service name: %s\n", project, location, serviceName)


	// `namespaces/{project_id_or_number}/services/{service_name}`
	// `projects/{project_id_or_number}/locations/{region}/services/{service_name}`
	// `projects/{project_id_or_number}/regions/{region}/services/{service_name}`
	// serviceFullName := fmt.Sprintf("namespaces/%s/services/%s", project, serviceName)
	serviceFullName := fmt.Sprintf("projects/%s/locations/%s/services/%s", project, location, serviceName)
	// serviceFullName := fmt.Sprintf("namespaces/%s/services/%s", project, serviceName)
	service, err := runService.Projects.Locations.Services.Get(serviceFullName).Do()
	if err != nil {
		return fmt.Errorf("failed to get service: %v", err)
	}

	service.Spec.Template.Metadata.Name = ""
	annotations := service.Spec.Template.Metadata.Annotations
	if _, exists := annotations["run.googleapis.com/cpu-throttling"]; exists {
		annotations["run.googleapis.com/cpu-throttling"] = "true"
		log.Printf("\"CPU is always allocated\" feature is ON !!!\n")
		log.Printf("\t -> Staring turning off \"CPU always allocated\" feature.\n")
	} else {
		log.Printf("\"CPU is always allocated\" feature is OFF\n")
		return nil
	}

	replcaeService, err := runService.Projects.Locations.Services.ReplaceService(serviceFullName, service).Do()
	if err != nil {
		return fmt.Errorf("failed to replace service: %v", err)
	}

	log.Println("Status: ", replcaeService.Status.Conditions)

	payloadStr := ""
	payloadStr += "MethodName: " + payload.MethodName + "\n"
	payloadStr += "ResourceName: " + payload.ResourceName + "\n"
	payloadStr += "RevisionName: " + payload.Response.Spec.Template.Metadata.Name + "\n"
	payloadStr += "*[WARNING] CPU is always allocated\" feature is ON !!!*" + "\n"

	err = sendToSlackChannel(&payloadStr)
	if err != nil {
		return fmt.Errorf("failed to post msg to slack: %v", err)
	}

	return nil
}
