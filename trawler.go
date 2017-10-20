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
	if command == "listSnapshots" {
		err = commandListSnapshots(ctx, client)
	} else if command == "snapshotVolume" {
		volumeName := flag.Arg(1)
		if volumeName == "" {
			fmt.Println("volumeName missed!")
			os.Exit(1)
		}
		region := flag.Arg(2)
		if region == "" {
			fmt.Println("region missed!")
			os.Exit(1)
		}
		var snapshotId string
		snapshotId, err = commandSnapshotVolume(ctx, client, volumeName, region, flag.Arg(3))
		if err == nil {
			fmt.Println("snapshot id: \"", snapshotId, "\" for volume: \""+volumeName+"\"  created:")
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
	} else {
		err = errors.New("unknown command: " + command)
	}

	if err != nil {
		os.Stderr.WriteString("ERROR: ", err.Error())
		os.Exit(1)
	}

}

func commandDeleteSnapshot(ctx context.Context, client *godo.Client, snapshotId string) error {
	_, err := client.Snapshots.Delete(ctx, snapshotId)
	return err
}

func commandSnapshotVolume(ctx context.Context, client *godo.Client, volumeName string, region string, snapshotName string) (string, error) {
	volumeId, err := getVolumeIdByVolumeName(ctx, client, volumeName, region)
	if err != nil {
		return "", err
	}
	if snapshotName == "" {
		timestamp := strconv.Itoa(int(time.Now().Unix()))
		snapshotName = volumeName + "-" + timestamp
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

func getVolumeIdByVolumeName(ctx context.Context, client *godo.Client, volumeName string, region string) (string, error) {

	options := &godo.ListOptions{}
	options.PerPage = 2
	params := &godo.ListVolumeParams{}
	params.Name = volumeName
	params.Region = region
	params.ListOptions = options

	volumes, _, err := client.Storage.ListVolumes(ctx, params)
	if err != nil {
		return "", err
	}
	if len(volumes) > 1 {
		return "", errors.New(strconv.Itoa(len(volumes)) + " volumes found with name " + volumeName + " please provide --region")
	}
	if len(volumes) == 0 {
		return "", errors.New("volume " + volumeName + " not found")
	}
	volumeId := volumes[0].ID
	return volumeId, nil
}

func commandListSnapshots(ctx context.Context, client *godo.Client) error {
	result, err := getSnapshotList(ctx, client)
	if err != nil {
		return err
	}

	fmt.Println("Snaphots found: ", len(result))
	for _, snapshot := range result {
		fmt.Println(snapshot.Name)
		// godo.Snapshot{ID:"28015723", Name:"git.hdws.de 2017-09-22", ResourceID:"5171268", ResourceType:"droplet", Regions:["fra1"], MinDiskSize:30, SizeGigaBytes:5.89, Created:"2017-09-22T22:39:52Z"}
		fmt.Printf("%v %v %v %v[%v] %vGB(%v)\n", snapshot.ResourceType, snapshot.ResourceID, snapshot.Created, snapshot.Name, snapshot.ID, snapshot.SizeGigaBytes, snapshot.MinDiskSize)
	}
	return nil
}

func help() string {
	var help string
	help += "Commands:\n"
	help += "listSnapshots: list all droplet and volume snapshots\n"
	help += "snapshotVolume VOLUMENAME REGION [SNAPSHOTNAME]: create snapshot of given volume\n"
	help += "deleteSnapshot SNAPSHOTID: delete snapshot\n"
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

func getSnapshotList(ctx context.Context, client *godo.Client) ([]godo.Snapshot, error) {

	list := []godo.Snapshot{}

	options := &godo.ListOptions{}
	options.PerPage = 1000
	for {
		snapshots, resp, err := client.Snapshots.List(ctx, options)
		if err != nil {
			return nil, err
		}
		// append our list
		for _, d := range snapshots {
			list = append(list, d)
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
