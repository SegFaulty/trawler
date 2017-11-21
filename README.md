# trawler
a simple retention manager for digital ocean snapshots (droplets and volumes)


## deployment

	go get -u github.com/SegFaulty/trawler
	go build github.com/SegFaulty/trawler
	mv trawler /usr/local/sbin/

## usage

### list accessable resources (droplets, volumes)
"token" is your digitalocean API-Token: see https://cloud.digitalocean.com/settings/api/tokens 

	./trawler -token 29e..6 listResources
	
### list all snapshots

	./trawler -token 29e..6 listSnapshots
	# list all snapshots for resource 
	./trawler -token 29e..6 listSnapshots 66579626
	
### cleanup (delete) snapshots for resource 

	# delete all but most recent snapshot of given resource
	./trawler -token 29e..6 cleanupSnapshots 66579626

	# delete all but the 3 most recent snapshot
	./trawler -token 29e..6 cleanupSnapshots 66579626 3
		
	# delete snapshots but keep the
	# 3 most recent (3r), the latest of this and last week (2w)
	# and 6 for the last months (6m) and on for this year and the last year (2y)
	./trawler -token 29e..6 cleanupSnapshots 66579626 2r2w6m2y
	
	# DRY mode, don't delete only show
	./trawler -dry -token 29e..6 cleanupSnapshots 66579626 2r2w6m2y
	
# retention strategy

use a combination of counts and timeperiod flags: 

- "r" keep the most recent n snapshots
- "y" keep the most recent n snapshots from n years
- "m" months
- "w" weeks
- "d" days

examples:
- 2r - keep the two most recent snapshots  
- 2r2y - keep the two most recent snapshots and one for this year and one for last year
         this will usually end in keeping 3 snapshots (the youngest two and the last of last year)

	