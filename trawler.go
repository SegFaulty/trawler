package main

import (
	"errors"
	"golang.org/x/net/context"
	"os"
	"strconv"
	"time"
	"github.com/digitalocean/godo"
	"golang.org/x/oauth2"
	"flag"
	"fmt"
	"regexp"
)

func main() {

	var token string
	flag.StringVar(&token, "token", "", "[REQUIRED] your digital ocean api token")
	flag.Parse()
	if token == "" {
		flag.Usage()
		os.Exit(1)
	}

	if len(flag.Args()) < 1 {
		fmt.Println("command missed!")
		print(help())
		os.Exit(1)
	}

	command := flag.Arg(0)

	tokenSource := &TokenSource{
		AccessToken: token,
	}
	oauthClient := oauth2.NewClient(context.Background(), tokenSource)
	client := godo.NewClient(oauthClient)

	ctx := context.TODO()

	var err error
	if command == "listResources" {
		err = commandListResources(ctx, client)
	} else if command == "listSnapshots" {
		resourceId := flag.Arg(1)
		err = commandListSnapshots(ctx, client, resourceId)
	} else if command == "snapshotVolume" {
		volumeId := flag.Arg(1)
		if volumeId == "" {
			fmt.Println("volumeId missed!")
			os.Exit(1)
		}

		var snapshotId string
		snapshotId, err = commandSnapshotVolume(ctx, client, volumeId, flag.Arg(2))
		if err == nil {
			fmt.Println("snapshot id: \"", snapshotId, "\" for volume: \""+ volumeId +"\"  created")
		}
	} else if command == "deleteSnapshot" {
		snapshotId := flag.Arg(1)
		if snapshotId == "" {
			fmt.Println("snapshotId missed!")
			os.Exit(1)
		}
		commandDeleteSnapshot(ctx, client, snapshotId)
		if err == nil {
			fmt.Println("snapshot id: \"", snapshotId, "\" deleted")
		}
	} else if command == "cleanupSnapshots" {
		resourceId := flag.Arg(1)
		if resourceId == "" {
			fmt.Println("recourceId missed!")
			os.Exit(1)
		}
		retentionString := flag.Arg(2)
		err = commandCleanupSnapshots(ctx, client, resourceId, retentionString)
		if err == nil {
			//fmt.Println("snapshot id: \"", snapshotId, "\" deleted")
		}
	} else {
		err = errors.New("unknown command: " + command)
	}

	if err != nil {
		os.Stderr.WriteString( "Error: " + err.Error() + "\n")
		os.Exit(1)
	}

}

func commandDeleteSnapshot(ctx context.Context, client *godo.Client, snapshotId string) error {
	_, err := client.Snapshots.Delete(ctx, snapshotId)
	return err
}

func commandSnapshotVolume(ctx context.Context, client *godo.Client, volumeId string, snapshotName string) (string, error) {
	volume, _, err := client.Storage.GetVolume(ctx, volumeId)
	if err != nil {
		return "", err
	}

	if snapshotName == "" {
		timestamp := strconv.Itoa(int(time.Now().Unix()))
		snapshotName = volume.Name + "-" + timestamp
	}
	request := &godo.SnapshotCreateRequest{}
	request.VolumeID = volumeId
	request.Name = snapshotName

	snapshot, _, err := client.Storage.CreateSnapshot(ctx, request)
	if err != nil {
		return "", err
	}
	return snapshot.ID, nil
}

func commandListSnapshots(ctx context.Context, client *godo.Client, resourceId string) error {
	result, err := getSnapshotList(ctx, client, resourceId)
	if err != nil {
		return err
	}

	fmt.Println("Snaphots found: ", len(result))
	for _, snapshot := range result {
		fmt.Println(snapshot.Name)
		fmt.Printf("  %v %v %v %v[%v] %vGB(%v)\n", snapshot.ResourceType, snapshot.ResourceID, snapshot.Created, snapshot.Name, snapshot.ID, snapshot.SizeGigaBytes, snapshot.MinDiskSize)
	}
	return nil
}

func commandListResources(ctx context.Context, client *godo.Client) error {

	type DoResource struct {
		resourceId string
		resourceType string
		name string
		region string
		regionCode string
		sizeGb string
	}

	resources := make([]DoResource,0)


	// get all droplets
	options := &godo.ListOptions{}
	options.PerPage = 1000
	for {
		droplets, resp, err := client.Droplets.List(ctx, options)
		if err != nil {
			return err
		}
		// append our list
		for _, droplet := range droplets {
			doResource := DoResource{}
			doResource.resourceType = "droplet";
			doResource.resourceId = strconv.Itoa(droplet.ID);
			doResource.name = droplet.Name;
			doResource.regionCode = droplet.Region.Slug;
			doResource.region = droplet.Region.Name;
			doResource.sizeGb = strconv.Itoa(droplet.Size.Disk);
			resources = append(resources, doResource)
		}

		// if we are at the last page, break out the for loop
		if resp.Links == nil || resp.Links.IsLastPage() {
			break
		}

		page, err := resp.Links.CurrentPage()
		if err != nil {
			return err
		}

		// set the page we want for the next request
		options.Page = page + 1
	}


	// get all Volumes / blockStorage
	options = &godo.ListOptions{}
	options.PerPage = 1000
	listVolumeParams := &godo.ListVolumeParams{}
	listVolumeParams.ListOptions = options
	for {
		volumes, resp, err := client.Storage.ListVolumes(ctx, listVolumeParams)
		if err != nil {
			return err
		}
		for _, v := range volumes {
			doResource := DoResource{}
			doResource.resourceType = "volume";
			doResource.resourceId = v.ID
			doResource.name = v.Name;
			doResource.region = v.Region.Name;
			doResource.regionCode = v.Region.Slug;
			doResource.sizeGb =  strconv.Itoa(int(v.SizeGigaBytes));
			resources = append(resources, doResource)
		}

		// if we are at the last page, break out the for loop
		if resp.Links == nil || resp.Links.IsLastPage() {
			break
		}

		page, err := resp.Links.CurrentPage()
		if err != nil {
			return err
		}

		// set the page we want for the next request
		options.Page = page + 1
	}


	fmt.Printf("========================================================\n",)
	fmt.Printf("%v | %v | %v | %v | %v \n", "ResourceType", "ResourceId", "Name", "Region", "DiskSize(GB)")
	fmt.Printf("========================================================\n",)
	for _, doResource := range resources {
		fmt.Printf("%v | %v | %v | %v (%v) | %v \n", doResource.resourceType, doResource.resourceId, doResource.name, doResource.region,  doResource.regionCode ,doResource.sizeGb)
	}

	return nil
}

func commandCleanupSnapshots(ctx context.Context, client *godo.Client, resourceId string, retentionString string) error {

	if retentionString == "" {
		retentionString = "1r"
	}
	// retentionString is simple numeric, so we take it as n-last
	if _, err := strconv.Atoi(retentionString); err == nil {
		retentionString = retentionString+"r"
	}

	fullStringRegexp := regexp.MustCompile("^(\\d+[rhdwmy])+$")
	if( !fullStringRegexp.MatchString(retentionString) ){
		return errors.New("invalid retention parameter")
	}


	snapshots, err := getSnapshotList(ctx, client, resourceId)
	if err != nil {
		return err
	}

	if len(snapshots)>0 {
		remainingSnapshotIds := make(map[string]bool)
		retentionRegExp := regexp.MustCompile("(\\d+)([rhdwmy]+)")
		for _, retentionElement := range retentionRegExp.FindAllStringSubmatch(retentionString, -1) {
			retentionType := retentionElement[2]
			retentionCount,_ := strconv.Atoi(retentionElement[1])

			switch retentionType {
			case "r":
				startIndex := len(snapshots) - retentionCount
				// emulate max(startIndex, 0)
				if startIndex<0 {
					startIndex = 0
				}
				endIndex := len(snapshots)
				remainingSnapshots := snapshots[startIndex : endIndex]
				for _, snapshot := range remainingSnapshots {
					remainingSnapshotIds[snapshot.ID] = true
				}
			default:
				return errors.New("invalid retentionType " + retentionType)
			}

		}

		for _, snapshot := range snapshots {
			if _,exists := remainingSnapshotIds[snapshot.ID]; exists == false{
				_, err := client.Snapshots.Delete(ctx, snapshot.ID)
				if err != nil {
					return err
				}

			}
		}
	}

	return nil
}


func help() string {
	var help string
	help += "Commands:\n"
	help += "listResources: list all digital ocean resources\n"
	help += "listSnapshots [RESOURCEID]: list all droplet and volume snapshots, or filtered to [RESOURCEID] \n"
	help += "snapshotVolume VOLUMEIID [SNAPSHOTNAME]: create snapshot of given volume\n"
	help += "deleteSnapshot SNAPSHOTID: delete snapshot\n"
	help += "cleanupSnapshots RESOURCEID [RETENTION]: delete all snapshots, not matching to retention-strategy. \n"
	help += "                 retention-strategy: default 1r, int will be converted to (n)r.\n"
	help += "                                  r: 'r' stands for 'recent'. So '3r' means 'keep the 3 most recent snapshots'.\n"
	return help
}

type TokenSource struct {
	AccessToken string
}

func (t *TokenSource) Token() (*oauth2.Token, error) {
	token := &oauth2.Token{
		AccessToken: t.AccessToken,
	}
	return token, nil
}

func getSnapshotList(ctx context.Context, client *godo.Client, resourceId string) ([]godo.Snapshot, error) {

	list := []godo.Snapshot{}

	options := &godo.ListOptions{}
	options.PerPage = 1000
	for {
		snapshots, resp, err := client.Snapshots.List(ctx, options)
		if err != nil {
			return nil, err
		}
		// append our list
		for _, snapshot := range snapshots {
			if resourceId=="" {
				list = append(list, snapshot)
			}else if resourceId==snapshot.ResourceID {
				list = append(list, snapshot)
			}
		}

		// if we are at the last page, break out the for loop
		if resp.Links == nil || resp.Links.IsLastPage() {
			break
		}

		page, err := resp.Links.CurrentPage()
		if err != nil {
			return nil, err
		}

		// set the page we want for the next request
		options.Page = page + 1
	}

	return list, nil
}
