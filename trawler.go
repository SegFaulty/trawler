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
	"text/tabwriter"
)

func main() {

	var token string
	flag.StringVar(&token, "token", "", "[REQUIRED] your digital ocean api token")
	var dryMode bool
	flag.BoolVar(&dryMode, "dry", false, "dry mode, don't delete only show")
	flag.Parse()
	if token == "" {
		flag.Usage()
		os.Exit(1)
	}

	if len(flag.Args()) < 1 {
		os.Stderr.WriteString( "command missed!" + "\n")
		print(help())
		os.Exit(1)
	}

	// warn if -dry is used in the wrong place
	for _, arg := range(flag.Args()) {
		if arg=="-dry" {
			os.Stderr.WriteString( "you have to use -dry flag before the first argument (sorry, this is go flag magic)" + "\n")
			os.Exit(1)
		}
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
		commandDeleteSnapshot(ctx, client, snapshotId, dryMode)
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
		err = commandCleanupSnapshots(ctx, client, resourceId, retentionString, dryMode)
	} else {
		err = errors.New("unknown command: " + command)
	}

	if err != nil {
		os.Stderr.WriteString( "Error: " + err.Error() + "\n")
		os.Exit(1)
	}

}

func commandDeleteSnapshot(ctx context.Context, client *godo.Client, snapshotId string, dryMode bool) error {
	if dryMode {
		fmt.Println("dry mode in effect: simulate delete " + snapshotId)
		return nil
	} else {
		_, err := client.Snapshots.Delete(ctx, snapshotId)
		return err
	}
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
	tabWriter := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprint(tabWriter, "TYPE\tRESOURCEID\tCREATED\tNAME[ID]\tSIZE\n")
	for _, snapshot := range result {
		fmt.Fprintf(tabWriter, "%v\t%v\t%v\t%v[%v]\t%vGB(%v)\n", snapshot.ResourceType, snapshot.ResourceID, snapshot.Created, snapshot.Name, snapshot.ID, snapshot.SizeGigaBytes, snapshot.MinDiskSize)
	}
	tabWriter.Flush()
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


	tabWriter := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tabWriter, "TYPE\tRESOURCEID\tNAME\tREGION\tDISKSIZE(GB)")
	for _, doResource := range resources {
		fmt.Fprintf(tabWriter, "%v\t%v\t%v\t%v (%v)\t%v\n", doResource.resourceType, doResource.resourceId, doResource.name, doResource.region,  doResource.regionCode ,doResource.sizeGb)
	}
	tabWriter.Flush()

	return nil
}

func commandCleanupSnapshots(ctx context.Context, client *godo.Client, resourceId string, retentionString string, dryMode bool) error {

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
		if dryMode {
			fmt.Println("snapshots found: "+strconv.Itoa(len(snapshots)))
		}
		remainingSnapshotIds := make(map[string]bool)
		retentionRegExp := regexp.MustCompile("(\\d+)([rdwmy]+)")
		for _, retentionElement := range retentionRegExp.FindAllStringSubmatch(retentionString, -1) {
			retentionType := retentionElement[2]
			retentionCount,_ := strconv.Atoi(retentionElement[1])

			if( retentionType=="r" ) {
				startIndex := len(snapshots) - retentionCount
				// emulate max(startIndex, 0)
				if startIndex < 0 {
					startIndex = 0
				}
				endIndex := len(snapshots)
				remainingSnapshots := snapshots[startIndex : endIndex]
				for _, snapshot := range remainingSnapshots {
					remainingSnapshotIds[snapshot.ID] = true
					if dryMode {
						fmt.Println(snapshot.Created + " (" + snapshot.ID + ") take it as retentionCount: " + strconv.Itoa(len(remainingSnapshotIds)) + " for retentionType: " + retentionType)
					}


				}
			}else{
				elementRemainingSnapshotIds, err := getRemainingSnapshotIds(snapshots, retentionType, retentionCount, dryMode)
				if err != nil {
					return err
				}
				for snapshotId := range elementRemainingSnapshotIds { // add to remaining list
					remainingSnapshotIds[snapshotId] = true
				}
			}

		}
		for _, snapshot := range snapshots {
			if _,exists := remainingSnapshotIds[snapshot.ID]; exists == false{
				if dryMode {
					fmt.Println("dry mode in effect: simulate delete " + snapshot.ID + " " + snapshot.Created)
				} else {
				_, err := client.Snapshots.Delete(ctx, snapshot.ID)
					if err != nil {
						return err
					}
				}
			}
		}
		if dryMode {
			fmt.Println("remaining snapshots: "+strconv.Itoa(len(remainingSnapshotIds)))
		}
	}

	return nil
}

func getRemainingSnapshotIds(snapshots []godo.Snapshot, retentionType string, retentionCount int, debug bool) (map[string]bool, error) {
	remainingSnapshotIds := make(map[string]bool)

	// build startTime endTime
	var startTime time.Time
	var endTime time.Time
	now := time.Now()
	switch retentionType {
	case "y": // year
		startTime = time.Date(now.Year(), 1,1,0,0,0,0,now.Location() )
		endTime = startTime.AddDate(1,0,0)
	case "m": // month
		startTime = time.Date(now.Year(), now.Month(),1,0,0,0,0,now.Location() )
		endTime = startTime.AddDate(0,1,0)
	case "d": // day
		startTime = time.Date(now.Year(), now.Month(),now.Day(),0,0,0,0,now.Location() )
		endTime = startTime.AddDate(0,0,1)
	case "w": // week
		startTime = time.Date(now.Year(), now.Month(),now.Day(),0,0,0,0,now.Location() )
		// iterate back to monday
		for startTime.Weekday() != time.Monday {
			startTime = startTime.AddDate(0, 0, -1)
		}
		endTime = startTime.AddDate(0,0,7)
	default:
		return nil, errors.New("invalid retentionType " + retentionType)
	}


	// get recent snapshot for every time period
	for currentCount :=1; currentCount<=retentionCount; currentCount++ {

		// iterate reverse, most recently snapshot first
		for index:=len(snapshots)-1; index>0 ; index-- {
			snapshot := snapshots[index]


			// get/parse snapshot time
			snapshotTimestamp, err := time.Parse("2006-01-02T15:04:05Z", snapshot.Created)
			if  err != nil {
				return nil, errors.New(fmt.Sprintf("ERROR: parse time %q resulted in error: %v\n", snapshot.Created, err))
			}
			// check if shnapshot in current time period
			if (snapshotTimestamp.Equal(startTime) || snapshotTimestamp.After(startTime)) &&  snapshotTimestamp.Before(endTime) {
				remainingSnapshotIds[snapshot.ID] = true
				if debug {
					fmt.Println(snapshot.Created + " (" + snapshot.ID + ") take it as retentionCount: " + strconv.Itoa(currentCount) + " for retentionType: " + retentionType)
				}
				break; // first (most recent) snapshot in this time period - take it, fo to next time period
			}
		}

		// calculate previous time period
		switch retentionType {
		case "y":
			startTime = startTime.AddDate(-1,0,0)
			endTime = startTime.AddDate(1,0,0)
		case "m":
			startTime = startTime.AddDate(0,-1,0)
			endTime = startTime.AddDate(0,1,0)
		case "d":
			startTime = startTime.AddDate(0,0,-1)
			endTime = startTime.AddDate(0,0,1)
		case "w":
			startTime = startTime.AddDate(0,0,-7)
			endTime = startTime.AddDate(0,0,7)
		}

	}

	return remainingSnapshotIds, nil
}


func help() string {
	var help string
	help += "https://github.com/SegFaulty/trawler:\n"
	help += "Commands:\n"
	help += "listResources: list all digital ocean resources accessable with this token\n"
	help += "listSnapshots [RESOURCEID]: list all droplet and volume snapshots, or filtered by [RESOURCEID] \n"
	help += "snapshotVolume VOLUMEID [SNAPSHOTNAME]: create snapshot of given volume\n"
	help += "deleteSnapshot SNAPSHOTID: delete snapshot\n"
	help += "cleanupSnapshots RESOURCEID [RETENTION]: delete all snapshots, not matching to retention-strategy. \n"
	help += "                 retention-strategy: default 1r, int will be converted to (n)r.\n"
	help += "                                  r: 'r' stands for 'recent'. So '3r' means 'keep the 3 most recent snapshots'.\n"
	help += "                                  y: 'r' stands for 'recent'. So '3r' means 'keep the 3 most recent snapshots'.\n"
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
